package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

type RelayOffer struct {
	TransferID string
	Channel    string
	Payload    string
	Sequence   int64
}

func (s *Service) LatestRelaySignalSequence(ctx context.Context) (int64, error) {
	var sequence int64
	err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),0) FROM peer_signals`).Scan(&sequence)
	return sequence, err
}

func (s *Service) RelayOffers(ctx context.Context, after int64) ([]RelayOffer, error) {
	rows, err := s.db.Query(ctx, `
        SELECT p.transfer_id,p.channel,p.payload,p.sequence FROM peer_signals p JOIN transfer_requests tr ON tr.id=p.transfer_id
        LEFT JOIN relay_offer_claims c ON c.sequence=p.sequence
        WHERE p.sequence>$1 AND p.channel IN ('relay-uplink-offer','relay-downlink-offer')
        AND tr.state IN ('PENDING','ACCEPTED','COMPLETED') AND tr.expires_at>clock_timestamp()
        AND (c.sequence IS NULL OR(c.completed_at IS NULL AND c.claimed_at<clock_timestamp()-interval '1 minute'))
        AND NOT EXISTS(SELECT 1 FROM peer_signals newer WHERE newer.transfer_id=p.transfer_id AND newer.channel=p.channel AND newer.sequence>p.sequence)
        ORDER BY p.sequence LIMIT 100`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var offers []RelayOffer
	for rows.Next() {
		var offer RelayOffer
		if err := rows.Scan(&offer.TransferID, &offer.Channel, &offer.Payload, &offer.Sequence); err != nil {
			return nil, err
		}
		offers = append(offers, offer)
	}
	return offers, rows.Err()
}

func (s *Service) ClaimRelayOffer(ctx context.Context, sequence int64, ownerID string) (bool, error) {
	result, err := s.db.Exec(ctx, `
		INSERT INTO relay_offer_claims (sequence, owner_id) VALUES ($1,$2)
		ON CONFLICT (sequence) DO UPDATE SET owner_id=EXCLUDED.owner_id, claimed_at=now()
		WHERE relay_offer_claims.completed_at IS NULL AND relay_offer_claims.claimed_at<now()-interval '1 minute'
	`, sequence, ownerID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (s *Service) GetTransferAny(ctx context.Context, id string) (Transfer, error) {
	return scanTransfer(s.db.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1`, id))
}

type RelayManager struct {
	service    *Service
	logger     *slog.Logger
	instanceID string
	mu         sync.Mutex
	active     map[string]*webrtc.PeerConnection
}

func NewRelayManager(service *Service, logger *slog.Logger) *RelayManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &RelayManager{service: service, logger: logger, instanceID: uuid.NewString(), active: map[string]*webrtc.PeerConnection{}}
}

func (m *RelayManager) Start(ctx context.Context) error {
	if m.service.RelayTransport() == "websocket" {
		<-ctx.Done()
		return nil
	}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	defer func() {
		m.mu.Lock()
		peers := make([]*webrtc.PeerConnection, 0, len(m.active))
		for _, p := range m.active {
			peers = append(peers, p)
		}
		m.mu.Unlock()
		for _, p := range peers {
			_ = p.Close()
		}
	}()
	for {
		// Durable, unfinished offers survive process restarts. Claims bound retries.
		offers, err := m.service.RelayOffers(ctx, 0)
		if err == nil {
			for _, offer := range offers {
				offer := offer
				go func() {
					if err := m.handleOffer(ctx, offer); err != nil && ctx.Err() == nil {
						m.logger.Debug("relay offer ended", "transfer_id", offer.TransferID, "error", err)
					}
				}()
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

func (m *RelayManager) Stop(context.Context) error { return nil }

func relayICEConfig() webrtc.Configuration {
	var servers []webrtc.ICEServer
	if value := os.Getenv("PINELINK_ICE_SERVERS"); value != "" {
		_ = json.Unmarshal([]byte(value), &servers)
	}
	return webrtc.Configuration{ICEServers: servers}
}

func (m *RelayManager) handleOffer(ctx context.Context, offer RelayOffer) error {
	claimed, err := m.service.ClaimRelayOffer(ctx, offer.Sequence, m.instanceID)
	if err != nil || !claimed {
		return err
	}
	answered := false
	defer func() {
		if !answered {
			release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_, _ = m.service.db.Exec(release, `DELETE FROM relay_offer_claims WHERE sequence=$1 AND owner_id=$2 AND completed_at IS NULL`, offer.Sequence, m.instanceID)
		}
	}()
	spool, err := m.service.prepareRelaySpool(ctx, offer.TransferID)
	if err != nil {
		return err
	}
	settings := webrtc.SettingEngine{}
	settings.SetSCTPMaxReceiveBufferSize(1 << 20)
	pc, err := webrtc.NewAPI(webrtc.WithSettingEngine(settings)).NewPeerConnection(relayICEConfig())
	if err != nil {
		return err
	}
	call, cancel := context.WithCancel(ctx)
	key := offer.TransferID + ":" + offer.Channel
	m.mu.Lock()
	prior := m.active[key]
	m.active[key] = pc
	m.mu.Unlock()
	if prior != nil {
		_ = prior.Close()
	}
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			cancel()
			m.mu.Lock()
			if m.active[key] == pc {
				delete(m.active, key)
			}
			m.mu.Unlock()
		}
	})
	keep := false
	defer func() {
		if !keep {
			cancel()
			_ = pc.Close()
		}
	}()
	var channelSeen atomic.Bool
	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() != "file" || channelSeen.Swap(true) {
			_ = channel.Close()
			return
		}
		channel.OnClose(cancel)
		if offer.Channel == "relay-uplink-offer" {
			channel.OnMessage(func(message webrtc.DataChannelMessage) {
				if message.IsString {
					return
				}
				if err := m.acceptUplink(call, spool.ID, message.Data); err != nil {
					m.logger.Debug("relay uplink stopped", "transfer_id", offer.TransferID, "error", err)
					cancel()
					_ = pc.Close()
				}
			})
		} else {
			channel.OnMessage(func(message webrtc.DataChannelMessage) {
				if message.IsString && string(message.Data) == "DONE" {
					cancel()
				}
			})
			go func() {
				if err := m.pumpDownlink(call, channel, spool); err != nil && call.Err() == nil {
					m.logger.Debug("relay downlink stopped", "transfer_id", offer.TransferID, "error", err)
				}
				cancel()
				_ = pc.Close()
			}()
		}
	})
	if err = pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.Payload}); err != nil {
		return err
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err = pc.SetLocalDescription(answer); err != nil {
		return err
	}
	select {
	case <-gathered:
	case <-call.Done():
		return call.Err()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("relay ICE gathering timed out")
	}
	answerChannel := "relay-uplink-answer"
	if offer.Channel == "relay-downlink-offer" {
		answerChannel = "relay-downlink-answer"
	}
	if err = m.service.answerRelayOffer(call, offer, m.instanceID, answerChannel, pc.LocalDescription().SDP); err != nil {
		return err
	}
	answered = true
	keep = true
	go m.watchConnection(call, cancel, pc, offer, spool.ID)
	return nil
}

func (m *RelayManager) watchConnection(ctx context.Context, cancel context.CancelFunc, pc *webrtc.PeerConnection, offer RelayOffer, uploadID string) {
	defer cancel()
	defer pc.Close()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		// Uplink and downlink can live on different instances. Only an offer
		// is claimed; newer negotiations supersede old connections everywhere.
		var allowed bool
		err := m.service.db.QueryRow(ctx, `SELECT ((tr.state IN ('PENDING','ACCEPTED') AND tr.expires_at>clock_timestamp()) OR
            (tr.state='COMPLETED' AND $3='relay-uplink-offer' AND (u.state='OPEN' OR u.last_activity_at>clock_timestamp()-interval '10 seconds')))
            AND u.state IN ('OPEN','SEALED')
            AND NOT EXISTS(SELECT 1 FROM peer_signals p WHERE p.transfer_id=tr.id AND p.channel=$3 AND p.sequence>$4)
            FROM transfer_requests tr JOIN upload_sessions u ON u.transfer_id=tr.id WHERE tr.id=$1 AND u.id=$2`, offer.TransferID, uploadID, offer.Channel, offer.Sequence).Scan(&allowed)
		if err != nil || !allowed {
			return
		}
	}
}

func (m *RelayManager) acceptUplink(ctx context.Context, uploadID string, frame []byte) error {
	if err := m.storeUplink(ctx, uploadID, frame); err != nil {
		return err
	}
	_, err := m.sealRelayIfComplete(ctx, uploadID)
	return err
}

func (m *RelayManager) storeUplink(ctx context.Context, uploadID string, frame []byte) error {
	index, data, err := decodeRelayFrame(frame)
	if err != nil {
		return err
	}
	for {
		tx, o, err := m.service.beginUploadObject(ctx, uploadID, true)
		if err != nil {
			return err
		}
		tx.Rollback(ctx)
		if o.State == "SEALED" {
			return nil
		}
		if o.Count == 0 {
			if index != 0 || len(data) != 0 {
				return ErrInvalid
			}
		} else {
			digest := sha256.Sum256(data)
			err = m.service.UploadChunk(ctx, uploadID, index, fmt.Sprintf("%x", digest), data)
			if isPaused(err) {
				if err = relayWait(ctx, 200*time.Millisecond); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (m *RelayManager) sealRelayIfComplete(ctx context.Context, uploadID string) (bool, error) {
	for {
		tx, o, err := m.service.beginUploadObject(ctx, uploadID, true)
		if err != nil {
			return false, err
		}
		var received int
		if o.State != "SEALED" {
			err = tx.QueryRow(ctx, `SELECT count(*) FROM upload_chunks WHERE upload_id=$1 AND storage_generation=$2`, uploadID, o.Generation).Scan(&received)
		}
		tx.Rollback(ctx)
		if err != nil {
			return false, err
		}
		if o.State == "SEALED" {
			return true, nil
		}
		if received != o.Count {
			return false, nil
		}
		_, err = m.service.CompleteUpload(ctx, uploadID)
		if isPaused(err) {
			if err = relayWait(ctx, 200*time.Millisecond); err != nil {
				return false, err
			}
			continue
		}
		return err == nil, err
	}
}

func isPaused(err error) bool {
	var conflict *StateConflictError
	if errors.As(err, &conflict) {
		if task, ok := conflict.Current.(Task); ok {
			return task.State == "PAUSED"
		}
	}
	return false
}

func relayWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type relayChannel interface {
	ReadyState() webrtc.DataChannelState
	BufferedAmount() uint64
	Send([]byte) error
}

func (m *RelayManager) pumpDownlink(ctx context.Context, channel relayChannel, spool uploadObject) error {
	for channel.ReadyState() != webrtc.DataChannelStateOpen {
		if channel.ReadyState() == webrtc.DataChannelStateClosed {
			return io.EOF
		}
		if err := relayWait(ctx, 10*time.Millisecond); err != nil {
			return err
		}
	}
	tr, err := m.service.GetTransferAny(ctx, *spool.TransferID)
	if err != nil {
		return err
	}
	next := int(tr.ReceivedBytes / int64(spool.ChunkSize))
	// Re-send the final partial chunk if a previous receiver stopped before its
	// receipt. Client disk checkpoints deduplicate it, then re-run verification.
	if next >= spool.Count && spool.Count > 0 {
		next = spool.Count - 1
	}
	count := max(1, spool.Count)
	err = orderedPipeline(ctx, next, count, relayPipelineWindow, func(read context.Context, index int) ([]byte, error) {
		for {
			data, err := m.service.readRelayChunk(read, spool.ID, index)
			if !errors.Is(err, ErrIncomplete) && !isPaused(err) {
				return data, err
			}
			if err := relayWait(read, 100*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}, func(index int, data []byte) error {
		for {
			if channel.BufferedAmount() > 256<<10 {
				if err := relayWait(ctx, 10*time.Millisecond); err != nil {
					return err
				}
				continue
			}
			// Prefetch is not an authorization cache. Check the current grant,
			// generation and task again at the actual send boundary.
			err := m.service.sendRelayFrame(ctx, spool, channel, encodeRelayFrame(index, data))
			if !isPaused(err) {
				return err
			}
			if err := relayWait(ctx, 100*time.Millisecond); err != nil {
				return err
			}
		}
	})
	if err != nil {
		return err
	}
	// Keep the connection alive for the receiver's final fsync/ack.
	<-ctx.Done()
	return ctx.Err()
}

func (s *Service) answerRelayOffer(ctx context.Context, offer RelayOffer, owner, channel, payload string) error {
	tx, tr, err := s.beginTransferObjects(ctx, offer.TransferID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = transferUploadAllowed(ctx, tx, tr); err != nil {
		return err
	}
	if offer.Channel == "relay-downlink-offer" && tr.State != "ACCEPTED" {
		return ErrConflict
	}
	var allowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay_offer_claims o
        WHERE o.sequence=$1 AND o.owner_id=$3 AND o.completed_at IS NULL AND o.claimed_at>clock_timestamp()-interval '1 minute'
        AND NOT EXISTS(SELECT 1 FROM peer_signals p WHERE p.transfer_id=$2 AND p.channel=$4 AND p.sequence>$1))`, offer.Sequence, offer.TransferID, owner, offer.Channel).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO peer_signals(transfer_id,sender_role,channel,payload,reply_to) VALUES($1,'RELAY',$2,$3,$4)`, offer.TransferID, channel, payload, offer.Sequence); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE relay_offer_claims SET completed_at=now() WHERE sequence=$1 AND owner_id=$2`, offer.Sequence, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func decodeRelayFrame(frame []byte) (int, []byte, error) {
	if len(frame) < 36 {
		return 0, nil, fmt.Errorf("short relay frame")
	}
	index := int(binary.BigEndian.Uint32(frame[:4]))
	data := frame[36:]
	digest := sha256.Sum256(data)
	if string(frame[4:36]) != string(digest[:]) {
		return 0, nil, fmt.Errorf("relay frame checksum mismatch")
	}
	return index, data, nil
}

func encodeRelayFrame(index int, data []byte) []byte {
	digest := sha256.Sum256(data)
	frame := make([]byte, 36+len(data))
	binary.BigEndian.PutUint32(frame, uint32(index))
	copy(frame[4:36], digest[:])
	copy(frame[36:], data)
	return frame
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
