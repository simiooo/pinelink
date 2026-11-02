package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/simiooo/pinelink/cli/internal/client"
)

func RunSender(ctx context.Context, api *client.Client, transfer client.Transfer, filePath, path string, timeout time.Duration) error {
	if path == "relay" {
		return runSenderPath(ctx, api, transfer, filePath, "relay", 30*time.Second)
	}
	for transfer.State == "PENDING" {
		var err error
		transfer, err = api.GetTransfer(ctx, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if transfer.State != "ACCEPTED" {
		return fmt.Errorf("P2P transfer is %s", transfer.State)
	}
	if path == "direct" || path == "auto" {
		err := runSenderPath(ctx, api, transfer, filePath, "direct", timeout)
		if err == nil {
			return nil
		}
		if path == "direct" {
			return err
		}
	}
	_ = api.AddP2PSignal(ctx, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID, "SENDER", "direct-failed-sender", "sender direct path failed")
	return runSenderPath(ctx, api, transfer, filePath, "relay", 30*time.Second)
}

func runSenderPath(ctx context.Context, api *client.Client, transfer client.Transfer, filePath, path string, timeout time.Duration) error {
	if path == "relay" {
		transport, err := api.RelayTransport(ctx, transfer.ID)
		if err != nil {
			return err
		}
		if transport == "websocket" {
			return runSocketSender(ctx, api, transfer, filePath)
		}
	}
	pc, err := newPeer()
	if err != nil {
		return err
	}
	defer pc.Close()
	channel, err := pc.CreateDataChannel("file", nil)
	if err != nil {
		return err
	}
	connectionErrors := senderErrors(pc, channel)
	offerChannel, answerChannel := directOffer, directAnswer
	if path == "relay" {
		offerChannel, answerChannel = uplinkOffer, uplinkAnswer
	}
	offerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := offer(offerCtx, api, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID, "SENDER", offerChannel, answerChannel, pc); err != nil {
		return err
	}
	if err := waitOpen(offerCtx, channel, connectionErrors); err != nil {
		return err
	}
	transferCtx, transferCancel := context.WithDeadline(ctx, transfer.ExpiresAt)
	defer transferCancel()
	finishAfterClose := func(cause error) error {
		poll, cancel := context.WithTimeout(transferCtx, 6*time.Second)
		defer cancel()
		current, err := api.GetP2PProgress(poll, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		if err == nil && ((path == "relay" && current.Completion.Stored) || (path == "direct" && current.Completion.Delivered)) {
			return nil
		}
		return cause
	}
	if path == "direct" {
		go func() {
			if _, err := waitSignal(transferCtx, api, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID, "direct-failed", 0); err == nil {
				select {
				case connectionErrors <- fmt.Errorf("receiver reported direct path failure"):
				default:
				}
			}
		}()
	}
	if err := sendFile(transferCtx, channel, filePath, connectionErrors); err != nil {
		return finishAfterClose(err)
	}
	for {
		poll, pollCancel := context.WithTimeout(transferCtx, 6*time.Second)
		current, err := api.GetP2PProgress(poll, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		pollCancel()
		if err != nil {
			select {
			case terminal := <-connectionErrors:
				return finishAfterClose(terminal)
			default:
			}
			return err
		}
		if (path == "relay" && current.Completion.Stored) || (path == "direct" && current.Completion.Delivered) {
			return nil
		}
		if current.State != "PENDING" && current.State != "ACCEPTED" && current.State != "COMPLETED" {
			return fmt.Errorf("P2P transfer ended in state %s", current.State)
		}
		if path == "relay" && (current.Spool.State == "FAILED" || current.Spool.State == "CANCELLED" || current.Spool.State == "UNAVAILABLE" || current.Spool.State == "EXPIRED") {
			return fmt.Errorf("relay spool is %s", current.Spool.State)
		}
		select {
		case err := <-connectionErrors:
			return finishAfterClose(err)
		case <-transferCtx.Done():
			return transferCtx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type fileSender interface {
	BufferedAmount() uint64
	Send([]byte) error
}

func sendFile(ctx context.Context, channel fileSender, path string, connectionErrors <-chan error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := make([]byte, 64<<10)
	index := 0
	for {
		select {
		case err := <-connectionErrors:
			return err
		default:
		}
		read, readErr := io.ReadFull(file, buffer)
		if read > 0 {
			for channel.BufferedAmount() > 256<<10 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-connectionErrors:
					return err
				case <-time.After(10 * time.Millisecond):
				}
			}
			if err := channel.Send(encodeFrame(index, buffer[:read])); err != nil {
				return err
			}
			index++
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if index == 0 {
		return channel.Send(encodeFrame(0, nil))
	}
	return nil
}

func RunReceiver(ctx context.Context, api *client.Client, transfer client.Transfer, partPath, path string, timeout time.Duration, check func() error, finalize func() error) error {
	probe, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	snapshot, err := api.GetP2PProgress(probe, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID)
	probeCancel()
	if err != nil {
		return err
	}
	epoch := snapshot.Route.Epoch + 1
	if epoch <= 0 {
		return fmt.Errorf("route epoch exhausted")
	}
	finalizing := false
	finish := func() error {
		finalizing = true
		if finalize != nil {
			return finalize()
		}
		return nil
	}
	if path == "direct" || path == "auto" {
		err := runReceiverPath(ctx, api, transfer, partPath, "direct", timeout, check, epoch, finish)
		if err == nil {
			return nil
		}
		if path == "direct" || finalizing {
			return err
		}
		epoch++
	}
	_ = api.AddP2PSignal(ctx, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, "RECEIVER", "direct-failed", "direct path failed")
	_ = os.Unsetenv("PINELINK_P2P_FAIL_AFTER_DIRECT")
	return runReceiverPath(ctx, api, transfer, partPath, "relay", 30*time.Second, check, epoch, finish)
}

func runReceiverPath(ctx context.Context, api *client.Client, transfer client.Transfer, partPath, path string, timeout time.Duration, check func() error, epoch int64, finalize func() error) error {
	ctx, stop := context.WithDeadline(ctx, transfer.ExpiresAt)
	defer stop()
	done := make(chan error, 1)
	reporter := newProgressReporter(ctx, 500*time.Millisecond, func() error {
		file, err := os.OpenFile(partPath, os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		defer file.Close()
		return file.Sync()
	}, func(call context.Context, n int64) error {
		return api.ObserveP2PProgress(call, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, n, strings.ToUpper(path), epoch)
	})
	defer reporter.Close()
	defer func() {
		flush, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = reporter.Flush(flush)
	}()
	configure := func(channel receiveChannel) {
		var frames sync.Mutex
		finished := false
		channel.OnMessage(func(message webrtc.DataChannelMessage) {
			frames.Lock()
			defer frames.Unlock()
			if finished {
				return
			}
			if err := receiveFrame(message.Data, partPath, transfer, check); err != nil {
				finished = true
				// Include the final durable prefix when a path fails after a successful write.
				if info, statErr := os.Stat(partPath); statErr == nil {
					reporter.Observe(info.Size())
				}
				select {
				case done <- err:
				default:
				}
				return
			}
			info, err := os.Stat(partPath)
			if err == nil {
				reporter.Observe(info.Size())
			}
			if err == nil && transfer.SizeBytes != nil && info.Size() == *transfer.SizeBytes {
				finished = true
				flush, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := reporter.Flush(flush)
				cancel()
				if err != nil {
					select {
					case done <- err:
					default:
					}
					return
				}
				if err := channel.SendText("DONE"); err != nil {
					select {
					case done <- err:
					default:
					}
					return
				}
				select {
				case done <- nil:
				default:
				}
			}
		})
	}
	transport := "webrtc"
	if path == "relay" {
		var err error
		transport, err = api.RelayTransport(ctx, transfer.ID)
		if err != nil {
			return err
		}
	}
	receiveCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if transport == "websocket" {
		channel, err := openSocketReceiver(ctx, api, transfer.ID, done)
		if err != nil {
			return err
		}
		defer channel.close()
		configure(channel)
	} else {
		pc, err := newPeer()
		if err != nil {
			return err
		}
		defer pc.Close()
		if path == "relay" {
			channel, err := pc.CreateDataChannel("file", nil)
			if err != nil {
				return err
			}
			configure(channel)
			if err := offer(receiveCtx, api, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, "RECEIVER", downlinkOffer, downlinkAnswer, pc); err != nil {
				return err
			}
		} else {
			pc.OnDataChannel(func(channel *webrtc.DataChannel) { configure(channel) })
			if err := answer(receiveCtx, api, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, "RECEIVER", directOffer, directAnswer, pc); err != nil {
				return err
			}
		}
	}
	doneCtx, doneCancel := context.WithDeadline(ctx, transfer.ExpiresAt)
	defer doneCancel()
	peerFailure := make(chan error, 1)
	if path != "relay" {
		go func() {
			if _, err := waitSignal(doneCtx, api, transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, "direct-failed-sender", 0); err == nil {
				peerFailure <- fmt.Errorf("sender reported direct path failure")
			}
		}()
	}
	select {
	case err := <-done:
		// Plaintext verification and receipt commit precede transport cleanup.
		if err == nil && finalize != nil {
			return finalize()
		}
		return err
	case err := <-peerFailure:
		return err
	case <-doneCtx.Done():
		return doneCtx.Err()
	}
}

func receiveFrame(frame []byte, partPath string, transfer client.Transfer, check func() error) error {
	index, data, err := decodeFrame(frame)
	if err != nil {
		return err
	}
	if check != nil {
		if err := check(); err != nil {
			return err
		}
	}
	if transfer.SizeBytes == nil {
		return fmt.Errorf("missing declared size")
	}
	offset := int64(index) * (64 << 10)
	expectedSize := int64(64 << 10)
	if remaining := *transfer.SizeBytes - offset; remaining < expectedSize {
		expectedSize = remaining
	}
	if offset > *transfer.SizeBytes || expectedSize < 0 || int64(len(data)) != expectedSize {
		return fmt.Errorf("invalid P2P chunk length")
	}
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > *transfer.SizeBytes {
		return fmt.Errorf("checkpoint exceeds declared file size")
	}
	if offset+int64(len(data)) <= info.Size() && (info.Size() > 0 || *transfer.SizeBytes == 0) {
		return nil
	}
	if offset != info.Size() {
		return fmt.Errorf("unexpected P2P chunk offset %d, expected %d", offset, info.Size())
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	received := info.Size() + int64(len(data))
	if transfer.SizeBytes != nil && received > *transfer.SizeBytes {
		return fmt.Errorf("received more than the declared file size")
	}
	if value := os.Getenv("PINELINK_P2P_FAIL_AFTER_DIRECT"); value != "" {
		threshold, err := strconv.ParseInt(value, 10, 64)
		if err == nil && received >= threshold {
			return fmt.Errorf("direct path failed after %d bytes", received)
		}
	}
	return nil
}

func VerifyPart(partPath string, transfer client.Transfer) error {
	if transfer.SizeBytes == nil || transfer.ContentRoot == nil {
		return fmt.Errorf("transfer metadata is incomplete")
	}
	file, err := os.Open(partPath)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return err
	}
	if size != *transfer.SizeBytes || hex.EncodeToString(digest.Sum(nil)) != *transfer.ContentRoot {
		return fmt.Errorf("P2P content checksum mismatch")
	}
	return nil
}
