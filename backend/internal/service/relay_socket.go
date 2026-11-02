package service

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

// RelaySocket uses the same fenced spool operations as the WebRTC relay. Only
// the transport changes; uploads remain separate from verified delivery.
type RelaySocket struct {
	manager *RelayManager
	spool   uploadObject
	owner   string
	device  string
	uplink  bool
	Expires time.Time
}

func (s *Service) PrepareRelaySocket(ctx context.Context, id, owner, device, role string) (*RelaySocket, error) {
	tr, err := s.GetTransfer(ctx, id, owner, device)
	if err != nil {
		return nil, err
	}
	uplink := role == "sender"
	if (uplink && (tr.SenderOwnerID != owner || tr.SenderDeviceID != device)) ||
		(!uplink && (role != "receiver" || tr.RecipientOwnerID != owner || tr.RecipientDeviceID != device || tr.State != "ACCEPTED")) {
		return nil, ErrNotFound
	}
	var active bool
	if err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE owner_id=$1 AND device_id=$2 AND status='ACTIVE')`, owner, device).Scan(&active); err != nil {
		return nil, err
	}
	if !active {
		return nil, ErrUnauthorized
	}
	spool, err := s.prepareRelaySpool(ctx, id)
	if err != nil {
		return nil, err
	}
	expires := tr.ExpiresAt
	if uplink && tr.State == "COMPLETED" {
		expires = s.now().Add(10 * time.Minute)
	}
	return &RelaySocket{manager: NewRelayManager(s, nil), spool: spool, owner: owner, device: device, uplink: uplink, Expires: expires}, nil
}

func (s *RelaySocket) Run(ctx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithDeadline(ctx, s.Expires)
	var workers sync.WaitGroup
	defer func() { cancel(); _ = conn.CloseNow(); workers.Wait() }()
	conn.SetReadLimit((64 << 10) + 36)
	workers.Add(1)
	go func() {
		defer workers.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			var allowed bool
			err := s.manager.service.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transfer_requests tr JOIN devices d ON d.owner_id=$2 AND d.device_id=$3
                JOIN upload_sessions u ON u.id=$5 AND u.transfer_id=tr.id
                WHERE tr.id=$1 AND d.status='ACTIVE' AND u.state IN ('OPEN','SEALED')
                AND((tr.state IN ('PENDING','ACCEPTED') AND tr.expires_at>clock_timestamp())
                OR(tr.state='COMPLETED' AND $4 AND(u.state='OPEN' OR u.last_activity_at>clock_timestamp()-interval '10 seconds'))))`, *s.spool.TransferID, s.owner, s.device, s.uplink, s.spool.ID).Scan(&allowed)
			if err != nil || !allowed {
				cancel()
				return
			}
		}
	}()
	if s.uplink {
		return s.receiveUplink(ctx, conn)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		_ = s.manager.pumpDownlink(ctx, socketChannel{ctx, conn}, s.spool)
		cancel()
	}()
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if kind == websocket.MessageText && string(data) == "DONE" {
			return nil
		}
		return ErrInvalid
	}
}

func (s *RelaySocket) receiveUplink(ctx context.Context, conn *websocket.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	jobs := make(chan []byte, relayPipelineWindow)
	progress := make(chan struct{}, 1)
	failed := make(chan error, 1)
	fail := func(err error) {
		select {
		case failed <- err:
		default:
		}
		cancel()
	}
	for range relayPipelineWindow {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case frame := <-jobs:
					if err := s.manager.storeUplink(ctx, s.spool.ID, frame); err != nil {
						fail(err)
						return
					}
					select {
					case progress <- struct{}{}:
					default:
					}
				}
			}
		}()
	}
	// One coordinator seals after all durable chunks arrive. Concurrent writes
	// cannot accidentally start one full-file verification per finishing worker.
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-progress:
				complete, err := s.manager.sealRelayIfComplete(ctx, s.spool.ID)
				if err != nil {
					fail(err)
					return
				}
				if complete {
					return
				}
			}
		}
	}()
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			select {
			case cause := <-failed:
				return cause
			default:
				return err
			}
		}
		if kind != websocket.MessageBinary {
			return ErrInvalid
		}
		select {
		case <-ctx.Done():
			select {
			case cause := <-failed:
				return cause
			default:
				return ctx.Err()
			}
		case jobs <- data:
		}
	}
}

type socketChannel struct {
	ctx  context.Context
	conn *websocket.Conn
}

func (c socketChannel) ReadyState() webrtc.DataChannelState {
	if c.ctx.Err() != nil {
		return webrtc.DataChannelStateClosed
	}
	return webrtc.DataChannelStateOpen
}
func (c socketChannel) BufferedAmount() uint64 { return 0 }
func (c socketChannel) Send(data []byte) error {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageBinary, data)
}
