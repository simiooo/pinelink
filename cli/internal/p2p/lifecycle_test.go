package p2p

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestSenderRejectsAChannelClosedBeforeNegotiation(t *testing.T) {
	pc, err := newPeer()
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	channel, err := pc.CreateDataChannel("file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = channel.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err = waitOpen(ctx, channel, senderErrors(pc, channel)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("closed channel waited for negotiation timeout: %v", err)
	}
}

func TestSenderObservesRemoteChannelCloseWhileICEIsConnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	settings := webrtc.SettingEngine{}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	settings.SetIPFilter(func(ip net.IP) bool { return ip.IsLoopback() })
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	sender, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	receiver, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	receiver.OnDataChannel(func(channel *webrtc.DataChannel) {
		channel.OnMessage(func(webrtc.DataChannelMessage) { _ = channel.Close() })
	})
	channel, err := sender.CreateDataChannel("file", nil)
	if err != nil {
		t.Fatal(err)
	}
	failures := senderErrors(sender, channel)
	exchange := func(from, to *webrtc.PeerConnection, answer bool) {
		t.Helper()
		gathered := webrtc.GatheringCompletePromise(from)
		var description webrtc.SessionDescription
		if answer {
			description, err = from.CreateAnswer(nil)
		} else {
			description, err = from.CreateOffer(nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = from.SetLocalDescription(description); err != nil {
			t.Fatal(err)
		}
		select {
		case <-gathered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if err = to.SetRemoteDescription(*from.LocalDescription()); err != nil {
			t.Fatal(err)
		}
	}
	exchange(sender, receiver, false)
	exchange(receiver, sender, true)
	if err = waitOpen(ctx, channel, failures); err != nil {
		t.Fatal(err)
	}
	if err = channel.Send([]byte("simulate failed relay write")); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-failures:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("wrong terminal signal: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("remote close left sender waiting for a storage checkpoint")
	}
}
