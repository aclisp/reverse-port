package main

import (
	"context"
	"io"
	"net"
	"sync"
)

func pipeBidirectional(ctx context.Context, a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go pipeOneWay(&wg, a, b)
	go pipeOneWay(&wg, b, a)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
	a.Close()
	b.Close()
	<-done
}

func pipeOneWay(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	closeWrite(dst)
}

func closeWrite(c net.Conn) {
	if tcp, ok := c.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		return
	}
	_ = c.Close()
}
