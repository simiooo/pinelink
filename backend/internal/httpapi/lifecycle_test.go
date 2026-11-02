package httpapi

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-kratos/kratos/v2/log"
)

func TestShutdownDrainsAnActiveHijackedSocket(t *testing.T) {
	server := NewServer("127.0.0.1:0", nil, log.NewStdLogger(io.Discard), false)
	entered, exited := make(chan struct{}), make(chan struct{})
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, finish, ok := server.api.beginRelay(r.Context())
		if !ok {
			w.WriteHeader(503)
			return
		}
		defer finish()
		defer close(exited)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		close(entered)
		_, _, _ = conn.Read(ctx)
	})
	endpoint, err := server.Endpoint()
	if err != nil {
		t.Fatal(err)
	}
	running := make(chan error, 1)
	go func() { running <- server.Start(t.Context()) }()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var conn *websocket.Conn
	for conn == nil && ctx.Err() == nil {
		conn, _, err = websocket.Dial(ctx, endpoint.String(), nil)
		if err != nil {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if conn == nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	<-entered
	if err := server.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("Stop returned before the socket handler exited")
	}
	if err := <-running; err != nil {
		t.Fatal(err)
	}
	if _, _, ok := server.api.beginRelay(t.Context()); ok {
		t.Fatal("accepted relay after shutdown")
	}
	if err := server.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}
