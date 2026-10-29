package socks5

import (
	"context"
	"net"
	"sync"
)

// watchContext closes conn when ctx is cancelled, so a blocked handshake
// cannot outlive the caller's context.
//
// The returned function stops the watcher and must be called when the
// handshake is done; otherwise a later cancellation would close a tunnel
// that has already been handed to the caller.
func watchContext(ctx context.Context, conn net.Conn) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	var mu sync.Mutex
	finished := false
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			mu.Lock()
			defer mu.Unlock()
			if finished {
				return
			}
			conn.Close()
		case <-done:
		}
	}()
	return func() {
		mu.Lock()
		finished = true
		mu.Unlock()
		close(done)
	}
}
