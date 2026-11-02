package p2p

import (
	"context"
	"fmt"
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
	for {
		poll, stop := context.WithTimeout(ctx, 6*time.Second)
		p, err := api.GetP2PProgress(poll, transfer.ID, transfer.SenderOwnerID, transfer.SenderDeviceID)
		stop()
		if err != nil {
			return err
		}
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
