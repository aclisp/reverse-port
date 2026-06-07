package main

import (
	"crypto/hmac"
	"fmt"
	"io"
	"strings"
)

const maxHeaderLength = 4096

type controlHeader struct {
	Token  string
	Remote string
	Target string
}

type dataHeader struct {
	Token string
	ID    string
}

func readHeader(r io.Reader) (string, error) {
	var b []byte
	var one [1]byte
	for len(b) < maxHeaderLength {
		n, err := r.Read(one[:])
		if n > 0 {
			if one[0] == '\n' {
				return strings.TrimSuffix(string(b), "\r"), nil
			}
			b = append(b, one[0])
		}
		if err != nil {
			if err == io.EOF && len(b) > 0 {
				return "", fmt.Errorf("unterminated header")
			}
			return "", err
		}
	}
	return "", fmt.Errorf("header too long")
}

func parseControlHeader(line string) (controlHeader, error) {
	parts := strings.Fields(line)
	if len(parts) != 4 || parts[0] != "CONTROL" {
		return controlHeader{}, fmt.Errorf("malformed control header")
	}
	h := controlHeader{Token: parts[1], Remote: parts[2], Target: parts[3]}
	if err := validateToken(h.Token); err != nil {
		return controlHeader{}, err
	}
	if _, err := normalizeRemoteAddress(h.Remote); err != nil {
		return controlHeader{}, err
	}
	if err := validateDialAddress(h.Target); err != nil {
		return controlHeader{}, err
	}
	return h, nil
}

func parseDataHeader(line string) (dataHeader, error) {
	parts := strings.Fields(line)
	if len(parts) != 3 || parts[0] != "DATA" {
		return dataHeader{}, fmt.Errorf("malformed data header")
	}
	h := dataHeader{Token: parts[1], ID: parts[2]}
	if err := validateToken(h.Token); err != nil {
		return dataHeader{}, err
	}
	if h.ID == "" || hasWhitespace(h.ID) {
		return dataHeader{}, fmt.Errorf("invalid connection id")
	}
	return h, nil
}

func writeControlHeader(w io.Writer, token, remote, target string) error {
	_, err := fmt.Fprintf(w, "CONTROL %s %s %s\n", token, remote, target)
	return err
}

func writeDataHeader(w io.Writer, token, id string) error {
	_, err := fmt.Fprintf(w, "DATA %s %s\n", token, id)
	return err
}

func writeOpen(w io.Writer, id string) error {
	_, err := fmt.Fprintf(w, "OPEN %s\n", id)
	return err
}

func tokenEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
