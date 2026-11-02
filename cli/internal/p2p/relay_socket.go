package p2p

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/simiooo/pinelink/cli/internal/client"
)

type socketSender struct {
	ctx  context.Context
	conn *websocket.Conn
}

func (c socketSender) BufferedAmount() uint64 { return 0 }
func (c socketSender) Send(data []byte) error {
	return c.conn.Write(c.ctx, websocket.MessageBinary, data)
}

func runSocketSender(ctx context.Context, api *client.Client, transfer client.Transfer, filePath string) error {
	ctx, cancel := context.WithDeadline(ctx, transfer.ExpiresAt)
	defer cancel()
	conn, err := api.DialRelay(ctx, transfer.ID, "sender")
	if err != nil {
		return err
	}
	errors := make(chan error, 1)
	readerDone := make(chan struct{})
	defer func() { cancel(); _ = conn.CloseNow(); <-readerDone }()
	conn.SetReadLimit(1024)
	go func() {
		defer close(readerDone)
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				errors <- err
				return
			}
		}
	}()
	checkpoint := func(cause error) error {
		poll, stop := context.WithTimeout(ctx, 6*time.Second)
		defer stop()
		p, err := api.GetP2PProgress(poll, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		if err == nil && p.Completion.Stored {
			return nil
		}
		return cause
	}
	if err = sendFile(ctx, socketSender{ctx, conn}, filePath, errors); err != nil {
		return checkpoint(err)
	}
	lastControl := time.Now()
	retryDelay := 250 * time.Millisecond
	for {
		poll, stop := context.WithTimeout(ctx, 6*time.Second)
		p, err := api.GetP2PProgress(poll, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		stop()
		if err != nil {
			// A transient control-plane timeout must not cancel a valid, still
			// connected upload while its cloud copy is being verified. Permission
			// errors fail immediately; the live socket and total outage are bounded.
			if transientRelayPoll(err) && time.Since(lastControl) < 30*time.Second {
				select {
				case cause := <-errors:
					return checkpoint(cause)
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(retryDelay):
				}
				retryDelay = min(2*time.Second, retryDelay*2)
				continue
			}
			return err
		}
		lastControl = time.Now()
		retryDelay = 250 * time.Millisecond
		if p.Completion.Stored {
			return nil
		}
		if (p.State != "PENDING" && p.State != "ACCEPTED" && p.State != "COMPLETED") || p.Spool.State == "FAILED" || p.Spool.State == "CANCELLED" || p.Spool.State == "EXPIRED" || p.Spool.State == "UNAVAILABLE" {
			return fmt.Errorf("relay spool is %s", p.Spool.State)
		}
		select {
		case err := <-errors:
			return checkpoint(err)
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func transientRelayPoll(err error) bool {
	var response *client.ResponseError
	if errors.As(err, &response) {
		return response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500
	}
	var network net.Error
	return errors.As(err, &network)
}

type receiveChannel interface {
	OnMessage(func(webrtc.DataChannelMessage))
	SendText(string) error
}

type socketReceiver struct {
	ctx      context.Context
	cancel   context.CancelFunc
	conn     *websocket.Conn
	terminal chan<- error
	done     chan struct{}
}

func openSocketReceiver(ctx context.Context, api *client.Client, id string, terminal chan<- error) (*socketReceiver, error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, err := api.DialRelay(ctx, id, "receiver")
	if err != nil {
		cancel()
		return nil, err
	}
	conn.SetReadLimit((64 << 10) + 36)
	return &socketReceiver{ctx, cancel, conn, terminal, make(chan struct{})}, nil
}
func (c *socketReceiver) OnMessage(receive func(webrtc.DataChannelMessage)) {
	go func() {
		defer close(c.done)
		for {
			kind, data, err := c.conn.Read(c.ctx)
			if err == nil && kind != websocket.MessageBinary {
				err = fmt.Errorf("unexpected relay frame")
			}
			if err != nil {
				select {
				case c.terminal <- err:
				default:
				}
				return
			}
			receive(webrtc.DataChannelMessage{Data: data})
		}
	}()
}
func (c *socketReceiver) SendText(text string) error {
	return c.conn.Write(c.ctx, websocket.MessageText, []byte(text))
}
func (c *socketReceiver) close() { c.cancel(); _ = c.conn.CloseNow(); <-c.done }
