package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/simiooo/pinelink/cli/internal/client"
)

const (
	directOffer    = "direct-offer"
	directAnswer   = "direct-answer"
	uplinkOffer    = "relay-uplink-offer"
	uplinkAnswer   = "relay-uplink-answer"
	downlinkOffer  = "relay-downlink-offer"
	downlinkAnswer = "relay-downlink-answer"
)

func newPeer() (*webrtc.PeerConnection, error) {
	var servers []webrtc.ICEServer
	if value := os.Getenv("PINELINK_ICE_SERVERS"); value != "" {
		if err := json.Unmarshal([]byte(value), &servers); err != nil {
			return nil, fmt.Errorf("parse PINELINK_ICE_SERVERS: %w", err)
		}
	} else if value := os.Getenv("PINELINK_STUN_URLS"); value != "" {
		servers = append(servers, webrtc.ICEServer{URLs: strings.Split(value, ",")})
	}
	return webrtc.NewPeerConnection(webrtc.Configuration{ICEServers: servers})
}

func offer(ctx context.Context, api *client.Client, transferID, ownerID, deviceID, role, offerChannel, answerChannel string, pc *webrtc.PeerConnection) error {
	gathered := webrtc.GatheringCompletePromise(pc)
	description, err := pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(description); err != nil {
		return err
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return ctx.Err()
	}
	sent, err := api.SubmitP2PSignal(ctx, transferID, ownerID, deviceID, role, offerChannel, pc.LocalDescription().SDP, 0)
	if err != nil {
		return err
	}
	signals, err := waitSignal(ctx, api, transferID, ownerID, deviceID, answerChannel, sent.Sequence, sent.Sequence)
	if err != nil {
		return err
	}
	return pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: signals.Payload})
}

func answer(ctx context.Context, api *client.Client, transferID, ownerID, deviceID, role, offerChannel, answerChannel string, pc *webrtc.PeerConnection) error {
	offer, err := waitSignal(ctx, api, transferID, ownerID, deviceID, offerChannel, 0)
	if err != nil {
		return err
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.Payload}); err != nil {
		return err
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	description, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(description); err != nil {
		return err
	}
	select {
	case <-gathered:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, err = api.SubmitP2PSignal(ctx, transferID, ownerID, deviceID, role, answerChannel, pc.LocalDescription().SDP, offer.Sequence)
	return err
}

func waitSignal(ctx context.Context, api *client.Client, transferID, ownerID, deviceID, channel string, after int64, replyTo ...int64) (client.P2PSignal, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		signals, err := api.P2PSignals(ctx, transferID, ownerID, deviceID, after)
		if err != nil {
			return client.P2PSignal{}, err
		}
		for _, signal := range signals.Signals {
			if signal.Channel == channel && (len(replyTo) == 0 || signal.ReplyTo == replyTo[0]) {
				return signal, nil
			}
			after = signal.Sequence
		}
		select {
		case <-ctx.Done():
			return client.P2PSignal{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Register before negotiation: a relay can close the data channel while ICE is
// still connected (for example when its storage write fails). Waiting only for
// ICE Failed leaves the sender polling an OPEN spool until expiry.
func senderErrors(pc *webrtc.PeerConnection, channel *webrtc.DataChannel) chan error {
	failures := make(chan error, 1)
	report := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	channel.OnClose(func() { report(fmt.Errorf("data channel closed before checkpoint: %w", io.ErrClosedPipe)) })
	channel.OnError(report)
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateFailed {
			report(fmt.Errorf("ICE connection ended in state %s", state))
		}
	})
	if state := channel.ReadyState(); state == webrtc.DataChannelStateClosing || state == webrtc.DataChannelStateClosed {
		report(io.ErrClosedPipe)
	}
	return failures
}

func waitOpen(ctx context.Context, channel *webrtc.DataChannel, failures <-chan error) error {
	if state := channel.ReadyState(); state == webrtc.DataChannelStateClosing || state == webrtc.DataChannelStateClosed {
		return io.ErrClosedPipe
	}
	if channel.ReadyState() == webrtc.DataChannelStateOpen {
		return nil
	}
	opened := make(chan struct{})
	channel.OnOpen(func() { close(opened) })
	select {
	case <-opened:
		return nil
	case err := <-failures:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func encodeFrame(index int, data []byte) []byte {
	digest := sha256.Sum256(data)
	frame := make([]byte, 36+len(data))
	binary.BigEndian.PutUint32(frame, uint32(index))
	copy(frame[4:36], digest[:])
	copy(frame[36:], data)
	return frame
}

func decodeFrame(frame []byte) (int, []byte, error) {
	if len(frame) < 36 {
		return 0, nil, fmt.Errorf("short P2P frame")
	}
	index := int(binary.BigEndian.Uint32(frame[:4]))
	data := frame[36:]
	digest := sha256.Sum256(data)
	if string(frame[4:36]) != string(digest[:]) {
		return 0, nil, fmt.Errorf("P2P frame checksum mismatch")
	}
	return index, data, nil
}

func waitConnected(ctx context.Context, pc *webrtc.PeerConnection) error {
	connected := make(chan struct{})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			close(connected)
		}
	})
	select {
	case <-connected:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
