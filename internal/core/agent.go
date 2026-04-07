// SPDX-License-Identifier: AGPL-3.0-or-later
package core

import (
	"fmt"
	"os"
)

// ResolveAgentID returns an identifier for the current agent/process.
// Uses MOTE_AGENT_ID env var if set, otherwise falls back to hostname-PID.
func ResolveAgentID() string {
	if id := os.Getenv("MOTE_AGENT_ID"); id != "" {
		return id
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
