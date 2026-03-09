package core

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestResolveAgentID_EnvVar(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "test-agent-42")
	id := ResolveAgentID()
	if id != "test-agent-42" {
		t.Fatalf("expected 'test-agent-42', got %q", id)
	}
}

func TestResolveAgentID_Fallback(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "")
	id := ResolveAgentID()

	hostname, _ := os.Hostname()
	expected := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	if id != expected {
		t.Fatalf("expected %q, got %q", expected, id)
	}
}

func TestResolveAgentID_FallbackFormat(t *testing.T) {
	t.Setenv("MOTE_AGENT_ID", "")
	id := ResolveAgentID()

	// Should contain a hyphen separating hostname and PID
	if !strings.Contains(id, "-") {
		t.Fatalf("fallback ID should contain '-', got %q", id)
	}
}
