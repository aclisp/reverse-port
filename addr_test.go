package main

import "testing"

func TestNormalizeRemoteAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"port only", "8080", "127.0.0.1:8080"},
		{"loopback", "127.0.0.1:8080", "127.0.0.1:8080"},
		{"empty host", ":8080", ":8080"},
		{"star", "*:8080", ":8080"},
		{"ipv4 any", "0.0.0.0:8080", "0.0.0.0:8080"},
		{"ipv6 loopback", "[::1]:8080", "[::1]:8080"},
		{"ipv6 any", "[::]:8080", "[::]:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRemoteAddress(tt.in)
			if err != nil {
				t.Fatalf("normalizeRemoteAddress() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeRemoteAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddressValidationRejectsInvalidInputs(t *testing.T) {
	for _, in := range []string{"", "0", "127.0.0.1:0", "tcp://127.0.0.1:80", "127.0.0.1:80 bad"} {
		if _, err := normalizeRemoteAddress(in); err == nil {
			t.Fatalf("normalizeRemoteAddress(%q) succeeded unexpectedly", in)
		}
	}
	for _, in := range []string{"", ":8080", "127.0.0.1:0", "tcp://127.0.0.1:80", "127.0.0.1:80 bad"} {
		if err := validateDialAddress(in); err == nil {
			t.Fatalf("validateDialAddress(%q) succeeded unexpectedly", in)
		}
	}
}

func TestValidateStatusListenLoopbackOnly(t *testing.T) {
	for _, in := range []string{"127.0.0.1:9001", "[::1]:9001"} {
		if err := validateStatusListen(in); err != nil {
			t.Fatalf("validateStatusListen(%q) error = %v", in, err)
		}
	}
	for _, in := range []string{":9001", "localhost:9001", "0.0.0.0:9001", "[::]:9001"} {
		if err := validateStatusListen(in); err == nil {
			t.Fatalf("validateStatusListen(%q) succeeded unexpectedly", in)
		}
	}
}

func TestValidateToken(t *testing.T) {
	for _, token := range []string{"", "has space", "has\ttab"} {
		if err := validateToken(token); err == nil {
			t.Fatalf("validateToken(%q) succeeded unexpectedly", token)
		}
	}
	if err := validateToken("secret"); err != nil {
		t.Fatalf("validateToken(secret) error = %v", err)
	}
}
