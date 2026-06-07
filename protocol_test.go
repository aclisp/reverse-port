package main

import (
	"strings"
	"testing"
)

func TestReadHeaderAndParse(t *testing.T) {
	line, err := readHeader(strings.NewReader("CONTROL secret 127.0.0.1:8080 localhost:3000\nignored"))
	if err != nil {
		t.Fatalf("readHeader error = %v", err)
	}
	h, err := parseControlHeader(line)
	if err != nil {
		t.Fatalf("parseControlHeader error = %v", err)
	}
	if h.Token != "secret" || h.Remote != "127.0.0.1:8080" || h.Target != "localhost:3000" {
		t.Fatalf("unexpected control header: %+v", h)
	}

	data, err := parseDataHeader("DATA secret abc123")
	if err != nil {
		t.Fatalf("parseDataHeader error = %v", err)
	}
	if data.ID != "abc123" {
		t.Fatalf("data id = %q, want abc123", data.ID)
	}
}

func TestProtocolRejectsBadHeaders(t *testing.T) {
	if _, err := readHeader(strings.NewReader(strings.Repeat("x", maxHeaderLength+1))); err == nil {
		t.Fatal("readHeader accepted overlong header")
	}
	if _, err := parseControlHeader("CONTROL bad token 127.0.0.1:8080 localhost:3000"); err == nil {
		t.Fatal("parseControlHeader accepted whitespace token")
	}
	if _, err := parseDataHeader("DATA secret"); err == nil {
		t.Fatal("parseDataHeader accepted short header")
	}
}
