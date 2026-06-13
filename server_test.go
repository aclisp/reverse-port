package main

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAddPendingIsAtomicWithCapacityCheck(t *testing.T) {
	tun := &tunnel{
		state:   &serverState{maxPendingConnections: 1, maxActiveConnections: 10},
		timeout: time.Second,
		pending: make(map[string]*pendingConn),
		active:  make(map[net.Conn]struct{}),
	}
	var successes atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if tun.addPending(fmt.Sprintf("id-%d", i), &pendingConn{}) {
				successes.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("addPending successes = %d, want 1", successes.Load())
	}
	for id, pc := range tun.pending {
		if pc.timer != nil {
			pc.timer.Stop()
		}
		delete(tun.pending, id)
	}
}

func TestActivatePairIsAtomicWithCapacityCheck(t *testing.T) {
	tun := &tunnel{
		state:   &serverState{maxPendingConnections: 10, maxActiveConnections: 1},
		pending: make(map[string]*pendingConn),
		active:  make(map[net.Conn]struct{}),
	}
	var successes atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		serverA, clientA := net.Pipe()
		serverB, clientB := net.Pipe()
		defer serverA.Close()
		defer clientA.Close()
		defer serverB.Close()
		defer clientB.Close()
		wg.Add(1)
		go func(a, b net.Conn) {
			defer wg.Done()
			<-start
			if tun.activatePair(a, b) {
				successes.Add(1)
			}
		}(serverA, serverB)
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("activatePair successes = %d, want 1", successes.Load())
	}
	tun.mu.Lock()
	defer tun.mu.Unlock()
	if got := tun.activeConnectionCountLocked(); got != 1 {
		t.Fatalf("activeConnectionCountLocked = %d, want 1", got)
	}
}
