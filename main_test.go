package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIValidation(t *testing.T) {
	var stderr bytes.Buffer
	code := runMain([]string{"client", "--server", "127.0.0.1:9000", "--remote", "8080", "--target", "127.0.0.1:3000"}, func(string) string { return "" }, nil, &stderr)
	if code != 2 {
		t.Fatalf("runMain code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage: rpf client") {
		t.Fatalf("stderr missing client usage: %s", stderr.String())
	}

	stderr.Reset()
	code = runMain([]string{"server", "--token", "secret", "--status-listen", "0.0.0.0:9001"}, func(string) string { return "" }, nil, &stderr)
	if code != 2 {
		t.Fatalf("runMain code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("stderr missing loopback error: %s", stderr.String())
	}
}

func TestParseClientFlagsNormalizesRemoteAndUsesEnvToken(t *testing.T) {
	cfg, ok := parseClientFlags([]string{"--server", "example.com:9000", "--remote", "8080", "--target", "localhost:3000"}, func(key string) string {
		if key == "RPORT_TOKEN" {
			return "secret"
		}
		return ""
	}, &bytes.Buffer{})
	if !ok {
		t.Fatal("parseClientFlags failed")
	}
	if cfg.Token != "secret" {
		t.Fatalf("token = %q, want secret", cfg.Token)
	}
	if cfg.Remote != "127.0.0.1:8080" {
		t.Fatalf("remote = %q, want normalized loopback", cfg.Remote)
	}
}
