package httpapi

import (
	"context"
	"errors"
	"net"
	"sync"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// net/http Shutdown does not drain hijacked WebSocket connections. Track them
// explicitly so the application cannot close its DB beneath a live relay.
type Server struct {
	*khttp.Server
	api     *API
	once    sync.Once
	drained chan struct{}
}

func wrapServer(server *khttp.Server, api *API) *Server {
	api.relayContext, api.relayCancel = context.WithCancel(context.Background())
	server.BaseContext = func(net.Listener) context.Context { return api.relayContext }
	return &Server{Server: server, api: api, drained: make(chan struct{})}
}

func (a *API) beginRelay(parent context.Context) (context.Context, func(), bool) {
	a.relayMutex.Lock()
	if a.relayStopping {
		a.relayMutex.Unlock()
		return nil, nil, false
	}
	a.relayWorkers.Add(1)
	a.relayMutex.Unlock()
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(a.relayContext, cancel)
	return ctx, func() { stop(); cancel(); a.relayWorkers.Done() }, true
}

func (s *Server) Stop(ctx context.Context) error {
	s.once.Do(func() {
		s.api.relayMutex.Lock()
		s.api.relayStopping = true
		s.api.relayMutex.Unlock()
		s.api.relayCancel()
		go func() { s.api.relayWorkers.Wait(); close(s.drained) }()
	})
	err := s.Server.Stop(ctx)
	select {
	case <-s.drained:
		return err
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}
