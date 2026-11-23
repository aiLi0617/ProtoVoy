//go:build !js

package mock

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

type Runtime struct {
	server  *http.Server
	cancel  context.CancelFunc
	done    chan error
	drained chan struct{}
	Address string
}

// Start runs a validated snapshot on a loopback listener. Port zero requests
// an available port from the operating system.
func Start(parent context.Context, address, version string, data []byte) (*Runtime, error) {
	addr, err := netip.ParseAddrPort(address)
	if err != nil || !addr.Addr().Unmap().IsLoopback() || addr.Addr().Zone() != "" {
		return nil, errors.New("Mock listener requires a numeric loopback address")
	}
	rules, err := Parse(data)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	app := &service{rules: rules, hits: make(map[string]uint64)}
	server := &http.Server{
		Handler:           app.handler(listener.Addr().String(), version),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	runtime := &Runtime{server: server, cancel: cancel, done: make(chan error, 1), drained: make(chan struct{}), Address: listener.Addr().String()}
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		runtime.done <- err
		close(runtime.done)
		cancel()
	}()
	go func() {
		defer close(runtime.drained)
		<-ctx.Done()
		deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if server.Shutdown(deadline) != nil {
			_ = server.Close()
		}
	}()
	return runtime, nil
}

func (runtime *Runtime) Close(ctx context.Context) error {
	runtime.cancel()
	select {
	case <-runtime.drained:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-runtime.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
