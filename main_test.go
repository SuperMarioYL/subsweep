package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The version a user sees comes from two places that must agree: the binary
// (embedded VERSION, or goreleaser's -ldflags stamp) and the recorded demo
// artifact that both READMEs quote verbatim. A forgotten bump used to ship a
// site and a demo that named the previous version.
func TestVersionMatchesVersionFile(t *testing.T) {
	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(raw))
	if want == "" {
		t.Fatal("VERSION is empty")
	}
	if version != want {
		t.Fatalf("binary version = %q, VERSION = %q", version, want)
	}

	raw, err = os.ReadFile("docs/demo-results.json")
	if err != nil {
		t.Fatalf("read docs/demo-results.json: %v", err)
	}
	var demo struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &demo); err != nil {
		t.Fatalf("parse docs/demo-results.json: %v", err)
	}
	if demo.Version != want {
		t.Fatalf("docs/demo-results.json version = %q, VERSION = %q", demo.Version, want)
	}
}
