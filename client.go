package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

type ClientConfig struct {
	Server            string
	Remote            string
	Target            string
	Token             string
	ReconnectInterval time.Duration
}

func RunClient(ctx context.Context, cfg ClientConfig, logger *log.Logger) error {
	if err := validateToken(cfg.Token); err != nil {
		return err
	}
	remote, err := normalizeRemoteAddress(cfg.Remote)
	if err != nil {
		return err
	}
	cfg.Remote = remote
	if err := validateDialAddress(cfg.Server); err != nil {
		return err
	}
	if err := validateDialAddress(cfg.Target); err != nil {
		return err
	}
	if cfg.ReconnectInterval <= 0 {
		return fmt.Errorf("reconnect interval must be positive")
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := runClientSession(ctx, cfg, logger); err != nil && ctx.Err() == nil {
			logger.Printf("client disconnected: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(cfg.ReconnectInterval):
		}
	}
}

func runClientSession(ctx context.Context, cfg ClientConfig, logger *log.Logger) error {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", cfg.Server)
	if err != nil {
		return err
	}
	defer conn.Close()
	enableKeepAlive(conn)
	if err := writeControlHeader(conn, cfg.Token, cfg.Remote, cfg.Target); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	resp, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	resp = strings.TrimSpace(resp)
	if resp != "OK" {
		return errors.New(resp)
	}
	logger.Printf("client connected to %s", cfg.Server)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		parts := strings.Fields(line)
		if len(parts) != 2 || parts[0] != "OPEN" {
			return fmt.Errorf("malformed server request")
		}
		id := parts[1]
		go handleOpen(ctx, cfg, logger, id)
	}
}

func handleOpen(ctx context.Context, cfg ClientConfig, logger *log.Logger, id string) {
	target, targetErr := (&net.Dialer{}).DialContext(ctx, "tcp", cfg.Target)
	if targetErr != nil {
		logger.Printf("target dial failed for %s: %v", cfg.Target, targetErr)
	}
	data, err := (&net.Dialer{}).DialContext(ctx, "tcp", cfg.Server)
	if err != nil {
		if targetErr == nil {
			target.Close()
		}
		return
	}
	enableKeepAlive(data)
	if err := writeDataHeader(data, cfg.Token, id); err != nil {
		data.Close()
		if targetErr == nil {
			target.Close()
		}
		return
	}
	if targetErr != nil {
		data.Close()
		return
	}
	enableKeepAlive(target)
	pipeBidirectional(ctx, target, data)
}
