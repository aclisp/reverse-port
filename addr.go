package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"
)

func validateToken(token string) error {
	if token == "" {
		return fmt.Errorf("token is required")
	}
	if hasWhitespace(token) {
		return fmt.Errorf("token must not contain whitespace")
	}
	return nil
}

func normalizeRemoteAddress(raw string) (string, error) {
	if err := validateAddressText(raw); err != nil {
		return "", err
	}
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("protocol-prefixed addresses are not supported")
	}
	if !strings.Contains(raw, ":") {
		if err := validatePort(raw); err != nil {
			return "", err
		}
		return net.JoinHostPort("127.0.0.1", raw), nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return "", err
	}
	if err := validatePort(port); err != nil {
		return "", err
	}
	if host == "*" {
		host = ""
	}
	return net.JoinHostPort(host, port), nil
}

func validateDialAddress(addr string) error {
	if err := validateAddressText(addr); err != nil {
		return err
	}
	if strings.Contains(addr, "://") {
		return fmt.Errorf("protocol-prefixed addresses are not supported")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" {
		return fmt.Errorf("host is required")
	}
	return validatePort(port)
}

func validateListenAddress(addr string, allowPortZero bool) error {
	if err := validateAddressText(addr); err != nil {
		return err
	}
	if strings.Contains(addr, "://") {
		return fmt.Errorf("protocol-prefixed addresses are not supported")
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 0 || p > 65535 {
		return fmt.Errorf("invalid port")
	}
	if p == 0 && !allowPortZero {
		return fmt.Errorf("port 0 is not supported")
	}
	return nil
}

func validateStatusListen(addr string) error {
	if err := validateListenAddress(addr, true); err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(addr)
	if host == "" {
		return fmt.Errorf("status listener must bind loopback")
	}
	ips, err := net.LookupIP(host)
	if err == nil && len(ips) > 0 {
		for _, ip := range ips {
			if ip.IsLoopback() {
				return nil
			}
		}
		return fmt.Errorf("status listener must bind loopback")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("status listener must bind loopback")
	}
	return nil
}

func validateAddressText(addr string) error {
	if addr == "" {
		return fmt.Errorf("address is required")
	}
	if hasWhitespace(addr) {
		return fmt.Errorf("address must not contain whitespace")
	}
	return nil
}

func validatePort(port string) error {
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("port must be 1-65535")
	}
	return nil
}

func hasWhitespace(s string) bool {
	return strings.IndexFunc(s, unicode.IsSpace) >= 0
}
