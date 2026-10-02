package main

import (
	"strings"
	"testing"
	"time"
)

func TestParsePlistStrings(t *testing.T) {
	raw := []byte(`<?xml version="1.0"?><plist><dict><key>CHALLENGE</key><string>abc</string><key>UDID</key><string>00000000-0000000000000000</string><key>PRODUCT</key><string>iPhone</string></dict></plist>`)
	got, err := parsePlistStrings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["CHALLENGE"] != "abc" || got["UDID"] != "00000000-0000000000000000" || got["PRODUCT"] != "iPhone" {
		t.Fatalf("parsed plist = %#v", got)
	}
}

func TestChallengeIsSingleUse(t *testing.T) {
	g := &gateway{challenges: map[string]time.Time{}}
	challenge, err := g.newChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if len(challenge) != 48 || strings.Trim(challenge, "0123456789abcdef") != "" {
		t.Fatalf("challenge = %q", challenge)
	}
	if !g.consumeChallenge(challenge) {
		t.Fatal("fresh challenge was rejected")
	}
	if g.consumeChallenge(challenge) {
		t.Fatal("challenge was accepted twice")
	}
}
