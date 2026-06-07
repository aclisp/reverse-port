package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
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

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}
