package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEndToEndForwardingAndStatus(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startEchoServer(t)
	defer closeTarget()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: 50 * time.Millisecond,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()
	waitForDial(t, remoteAddr)

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial remote: %v", err)
	}
	fmt.Fprintln(conn, "ping")
	got, err := bufio.NewReader(conn).ReadString('\n')
	conn.Close()
	if err != nil {
		t.Fatalf("read forwarded response: %v", err)
	}
	if got != "echo:ping\n" {
		t.Fatalf("forwarded response = %q", got)
	}

	resp, err := http.Get("http://" + statusAddr + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()
	var status statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "ok" || status.Current.ActiveTunnels != 1 || len(status.Tunnels) != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
	if strings.Contains(fmt.Sprintf("%+v", status), "secret") {
		t.Fatalf("status leaked token: %+v", status)
	}
}

func TestClientRetriesAfterRemoteBindConflict(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startEchoServer(t)
	defer closeTarget()

	blocker, err := net.Listen("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("block remote listen: %v", err)
	}

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: 30 * time.Millisecond,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()

	time.Sleep(120 * time.Millisecond)
	blocker.Close()
	waitForDial(t, remoteAddr)
}

func TestTargetFailureClosesRemoteCaller(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	missingTarget := freeTCPAddr(t)

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            missingTarget,
			Token:             "secret",
			ReconnectInterval: 50 * time.Millisecond,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()
	waitForDial(t, remoteAddr)

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial remote: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadAll(conn)
	if err == nil {
		return
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatalf("remote caller did not close after target failure")
	}
}

func TestClientStopsOnContextCancel(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startEchoServer(t)
	defer closeTarget()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: time.Second,
		}, testLogger(t))
	}()
	waitForDial(t, remoteAddr)

	stopClient()
	select {
	case err := <-clientDone:
		if err != nil {
			t.Fatalf("RunClient returned error after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunClient did not stop after context cancellation")
	}
}

func TestServerCancelDoesNotLogStatusAcceptClosed(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	logs := &syncBuffer{}

	serverCtx, stopServer := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, log.New(logs, "", 0))
	}()
	waitForDial(t, serverAddr)

	stopServer()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("RunServer returned error after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunServer did not stop after context cancellation")
	}
	if strings.Contains(logs.String(), "status server stopped") || strings.Contains(logs.String(), "use of closed network connection") {
		t.Fatalf("server logged expected shutdown noise: %s", logs.String())
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestServerCancelClosesActiveTunnelRemoteListener(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startEchoServer(t)
	defer closeTarget()

	serverCtx, stopServer := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- RunServer(serverCtx, ServerConfig{
			Listen:       serverAddr,
			StatusListen: statusAddr,
			OpenTimeout:  time.Second,
			Token:        "secret",
		}, testLogger(t))
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: time.Second,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()
	waitForDial(t, remoteAddr)

	stopServer()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatalf("RunServer returned error after cancel: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunServer did not stop after context cancellation")
	}
	waitForDialFailure(t, remoteAddr)
}

func TestInitialHeaderReadTimesOut(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	start := time.Now()
	_, err := readInitialHeader(serverSide, 30*time.Millisecond)
	if err == nil {
		t.Fatal("readInitialHeader succeeded unexpectedly")
	}
	if time.Since(start) > time.Second {
		t.Fatal("readInitialHeader did not time out promptly")
	}
}

func TestHeartbeatKeepsResponsiveClientTunnelAlive(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startEchoServer(t)
	defer closeTarget()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:            serverAddr,
			StatusListen:      statusAddr,
			OpenTimeout:       time.Second,
			HeartbeatInterval: 20 * time.Millisecond,
			HeartbeatTimeout:  200 * time.Millisecond,
			Token:             "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: 50 * time.Millisecond,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()
	waitForDial(t, remoteAddr)
	time.Sleep(90 * time.Millisecond)

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial remote after heartbeats: %v", err)
	}
	fmt.Fprintln(conn, "ping")
	got, err := bufio.NewReader(conn).ReadString('\n')
	conn.Close()
	if err != nil {
		t.Fatalf("read forwarded response after heartbeats: %v", err)
	}
	if got != "echo:ping\n" {
		t.Fatalf("forwarded response = %q", got)
	}
}

func TestHeartbeatTimeoutClosesStaleTunnel(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr := freeTCPAddr(t)

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:            serverAddr,
			StatusListen:      statusAddr,
			OpenTimeout:       time.Second,
			HeartbeatInterval: 20 * time.Millisecond,
			HeartbeatTimeout:  50 * time.Millisecond,
			Token:             "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	control, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("dial control: %v", err)
	}
	defer control.Close()
	if err := writeControlHeader(control, "secret", remoteAddr, targetAddr); err != nil {
		t.Fatalf("write control: %v", err)
	}
	resp, err := bufio.NewReader(control).ReadString('\n')
	if err != nil {
		t.Fatalf("read control response: %v", err)
	}
	if strings.TrimSpace(resp) != "OK" {
		t.Fatalf("control response = %q, want OK", resp)
	}
	waitForDial(t, remoteAddr)
	waitForDialFailure(t, remoteAddr)
}

func TestPendingConnectionCapRejectsExtraRemoteCallers(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr := freeTCPAddr(t)

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:                serverAddr,
			StatusListen:          statusAddr,
			OpenTimeout:           time.Second,
			MaxPendingConnections: 1,
			MaxActiveConnections:  10,
			Token:                 "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	control, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("dial control: %v", err)
	}
	defer control.Close()
	if err := writeControlHeader(control, "secret", remoteAddr, targetAddr); err != nil {
		t.Fatalf("write control: %v", err)
	}
	controlReader := bufio.NewReader(control)
	resp, err := controlReader.ReadString('\n')
	if err != nil {
		t.Fatalf("read control response: %v", err)
	}
	if strings.TrimSpace(resp) != "OK" {
		t.Fatalf("control response = %q, want OK", resp)
	}
	waitForDial(t, remoteAddr)

	firstRemote, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial first remote: %v", err)
	}
	defer firstRemote.Close()
	firstOpen, err := controlReader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first OPEN: %v", err)
	}
	if !strings.HasPrefix(firstOpen, "OPEN ") {
		t.Fatalf("first control line = %q, want OPEN", firstOpen)
	}

	secondRemote, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial second remote: %v", err)
	}
	defer secondRemote.Close()
	secondRemote.SetReadDeadline(time.Now().Add(time.Second))
	_, err = secondRemote.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("second remote caller stayed open unexpectedly")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("second remote caller was not closed after pending cap")
	}
}

func TestActiveConnectionCapRejectsExtraDataAttach(t *testing.T) {
	serverAddr := freeTCPAddr(t)
	statusAddr := freeTCPAddr(t)
	remoteAddr := freeTCPAddr(t)
	targetAddr, closeTarget := startHoldingServer(t)
	defer closeTarget()

	serverCtx, stopServer := context.WithCancel(context.Background())
	defer stopServer()
	go func() {
		err := RunServer(serverCtx, ServerConfig{
			Listen:                serverAddr,
			StatusListen:          statusAddr,
			OpenTimeout:           time.Second,
			MaxPendingConnections: 10,
			MaxActiveConnections:  1,
			Token:                 "secret",
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunServer error = %v", err)
		}
	}()
	waitForDial(t, serverAddr)

	clientCtx, stopClient := context.WithCancel(context.Background())
	defer stopClient()
	go func() {
		err := RunClient(clientCtx, ClientConfig{
			Server:            serverAddr,
			Remote:            remoteAddr,
			Target:            targetAddr,
			Token:             "secret",
			ReconnectInterval: 50 * time.Millisecond,
		}, testLogger(t))
		if err != nil {
			t.Errorf("RunClient error = %v", err)
		}
	}()
	waitForDial(t, remoteAddr)

	firstRemote, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial first remote: %v", err)
	}
	defer firstRemote.Close()
	waitForStatus(t, statusAddr, func(status statusResponse) bool {
		return status.Current.ActiveConnections == 1
	})

	secondRemote, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		t.Fatalf("dial second remote: %v", err)
	}
	defer secondRemote.Close()
	secondRemote.SetReadDeadline(time.Now().Add(time.Second))
	_, err = secondRemote.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("second active remote stayed open unexpectedly")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("second active remote was not closed after active cap")
	}
}

func startEchoServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadString('\n')
				if err == nil {
					fmt.Fprint(conn, "echo:"+line)
				}
			}()
		}
	}()
	return ln.Addr().String(), func() {
		cancel()
		ln.Close()
		<-ctx.Done()
	}
}

func startHoldingServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen holding server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				conn.Close()
			}()
		}
	}()
	return ln.Addr().String(), func() {
		ln.Close()
		<-done
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve addr: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func waitForDial(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", addr)
}

func waitForDialFailure(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to stop accepting connections", addr)
}

func waitForStatus(t *testing.T, statusAddr string, ok func(statusResponse) bool) statusResponse {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last statusResponse
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + statusAddr + "/status")
		if err == nil {
			func() {
				defer resp.Body.Close()
				_ = json.NewDecoder(resp.Body).Decode(&last)
			}()
			if ok(last) {
				return last
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status condition, last = %+v", last)
	return last
}

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}
