package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pion/webrtc/v4"
)

func TestSocketUplinkBoundsStorageAndSealsOnce(t *testing.T) {
	s, _, blobs := testStorage(t)
	data := make([]byte, (relayPipelineWindow+3)*(64<<10))
	for i := range data {
		data[i] = byte(i * 31)
	}
	tr := newTestTransfer(t, s, data, false)
	socket, err := s.PrepareRelaySocket(t.Context(), tr.ID, tr.SenderOwnerID, tr.SenderDeviceID, "sender")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	entered := make(chan struct{}, relayPipelineWindow)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var active, peak, reads atomic.Int32
	blobs.putGate = func(string) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			p := peak.Load()
			if p >= n || peak.CompareAndSwap(p, n) {
				break
			}
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}
	blobs.getGate = func(string) { reads.Add(1) }
	done := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			done <- err
			return
		}
		done <- socket.Run(ctx, conn)
	}))
	defer func() { releaseOnce.Do(func() { close(release) }); cancel(); server.Close() }()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	sendDone := make(chan error, 1)
	go func() {
		for i := 0; i < len(data)/(64<<10); i++ {
			if err := conn.Write(ctx, websocket.MessageBinary, encodeRelayFrame(i, data[i*(64<<10):(i+1)*(64<<10)])); err != nil {
				sendDone <- err
				return
			}
		}
		sendDone <- nil
	}()
	for range relayPipelineWindow {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if peak.Load() != relayPipelineWindow {
		t.Fatal("storage concurrency", peak.Load())
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-sendDone; err != nil {
		t.Fatal(err)
	}
	for {
		progress, err := s.GetP2PProgress(ctx, tr.ID, tr.SenderOwnerID, tr.SenderDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if progress.Completion.Stored {
			break
		}
		if err := relayWait(ctx, 10*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if reads.Load() != int32(len(data)/(64<<10)) {
		t.Fatal("duplicated full-file verification", reads.Load())
	}
	_ = conn.CloseNow()
	<-done
	if active.Load() != 0 || peak.Load() > relayPipelineWindow {
		t.Fatal("unjoined/unbounded writes")
	}
}

type gatedRelayChannel struct {
	first                   sync.Once
	entered, release, retry chan struct{}
	retryOnce               sync.Once
	checks                  atomic.Int32
	sent                    atomic.Int32
}

func (c *gatedRelayChannel) ReadyState() webrtc.DataChannelState { return webrtc.DataChannelStateOpen }
func (c *gatedRelayChannel) BufferedAmount() uint64 {
	c.first.Do(func() { close(c.entered); <-c.release })
	if c.checks.Add(1) > 1 {
		c.retryOnce.Do(func() { close(c.retry) })
	}
	return 0
}
func (c *gatedRelayChannel) Send([]byte) error { c.sent.Add(1); return nil }

func TestPrefetchedRelayFramesRespectCurrentAuthority(t *testing.T) {
	for _, control := range []string{"cancel", "device-revoke", "grant-expiry", "pause", "generation"} {
		t.Run(control, func(t *testing.T) {
			s, _, _ := testStorage(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			data := make([]byte, relayPipelineWindow*(64<<10))
			tr := newTestTransfer(t, s, data, false)
			spool, err := s.prepareRelaySpool(ctx, tr.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < spool.Count; i++ {
				part := data[i*spool.ChunkSize : (i+1)*spool.ChunkSize]
				if err = s.UploadChunk(ctx, spool.ID, i, digest(part), part); err != nil {
					t.Fatal(err)
				}
			}
			tr = transferAction(t, s, tr, "accept")
			channel := &gatedRelayChannel{entered: make(chan struct{}), release: make(chan struct{}), retry: make(chan struct{})}
			var once sync.Once
			defer once.Do(func() { close(channel.release) })
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() { defer close(finished); done <- NewRelayManager(s, nil).pumpDownlink(ctx, channel, spool) }()
			t.Cleanup(func() {
				cancel()
				once.Do(func() { close(channel.release) })
				select {
				case <-finished:
				case <-time.After(3 * time.Second):
					t.Error("relay pipeline did not stop")
				}
			})
			select {
			case <-channel.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			for {
				var completed int
				if err = s.db.QueryRow(ctx, `SELECT count(*) FROM object_io_operations WHERE upload_id=$1 AND kind='READ' AND state='CLOSED'`, spool.ID).Scan(&completed); err != nil {
					t.Fatal(err)
				}
				if completed == relayPipelineWindow {
					break
				}
				if err = relayWait(ctx, 5*time.Millisecond); err != nil {
					t.Fatal(err)
				}
			}
			switch control {
			case "cancel":
				transferAction(t, s, tr, "cancel")
			case "device-revoke":
				if _, err = s.RevokeDevice(ctx, DeviceRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID}); err != nil {
					t.Fatal(err)
				}
			case "grant-expiry":
				mustSQL(t, s, `UPDATE file_access_grants SET expires_at=clock_timestamp()-interval '1 second' WHERE transfer_id=$1`, tr.ID)
			case "pause":
				mustSQL(t, s, `UPDATE tasks SET state='PAUSED' WHERE reference_id=$1 AND kind='DOWNLOAD'`, tr.ID)
			case "generation":
				mustSQL(t, s, `UPDATE file_versions SET storage_generation=gen_random_uuid() WHERE id=$1`, spool.VersionID)
			}
			once.Do(func() { close(channel.release) })
			if control == "pause" {
				select {
				case <-channel.retry:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("revoked pump succeeded")
				}
			case <-ctx.Done():
				if control != "pause" {
					t.Fatal(ctx.Err())
				}
				<-done
			}
			if channel.sent.Load() != 0 {
				t.Fatal("prefetched data bypassed current authority", control)
			}
		})
	}
}

type admissionRelayChannel struct {
	entered, release chan struct{}
	enterOnce        sync.Once
}

func (c *admissionRelayChannel) ReadyState() webrtc.DataChannelState {
	return webrtc.DataChannelStateOpen
}
func (c *admissionRelayChannel) BufferedAmount() uint64 { return 0 }
func (c *admissionRelayChannel) Send([]byte) error {
	c.enterOnce.Do(func() { close(c.entered) })
	<-c.release
	return nil
}

func TestRelaySendAndDeviceRevocationHaveOneAdmissionOrder(t *testing.T) {
	for _, role := range []string{"sender", "receiver"} {
		t.Run(role, func(t *testing.T) {
			s, _, _ := testStorage(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			data := []byte("prefetched ciphertext")
			tr := newTestTransfer(t, s, data, false)
			spool := spoolData(t, s, tr, data)
			tr = transferAction(t, s, tr, "accept")
			cfg := s.db.Config()
			name := "sim883-revoke-" + uuid.NewString()
			cfg.ConnConfig.RuntimeParams["application_name"] = name
			cfg.MaxConns = 1
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			revoker := *s
			revoker.db = pool
			channel := &admissionRelayChannel{entered: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			defer func() { once.Do(func() { close(channel.release) }); cancel(); pool.Close() }()
			sendDone := make(chan error, 1)
			go func() { sendDone <- s.sendRelayFrame(ctx, spool, channel, encodeRelayFrame(0, data)) }()
			select {
			case <-channel.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			owner, device := tr.SenderOwnerID, tr.SenderDeviceID
			if role == "receiver" {
				owner, device = tr.RecipientOwnerID, tr.RecipientDeviceID
			}
			revokeDone := make(chan error, 1)
			go func() {
				_, err := revoker.RevokeDevice(ctx, DeviceRequest{OwnerID: owner, DeviceID: device})
				revokeDone <- err
			}()
			for {
				var waiting bool
				err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, name).Scan(&waiting)
				if err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-revokeDone:
					t.Fatal("revocation returned during admitted send", err)
				default:
				}
				if err = relayWait(ctx, 5*time.Millisecond); err != nil {
					t.Fatal(err)
				}
			}
			once.Do(func() { close(channel.release) })
			if err = <-sendDone; err != nil {
				t.Fatal(err)
			}
			if err = <-revokeDone; err != nil {
				t.Fatal(err)
			}
			if err = s.sendRelayFrame(ctx, spool, channel, encodeRelayFrame(0, data)); err == nil {
				t.Fatal("post-revocation frame was admitted")
			}
		})
	}
}
