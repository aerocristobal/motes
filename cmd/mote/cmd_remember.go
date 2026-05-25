// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"motes/internal/core"
	"motes/internal/security"
)

var rememberCmd = &cobra.Command{
	Use:   "remember <text>",
	Short: "Save a durable rule (memory) auto-injected into mote prime",
	Long: `Save a short durable rule (a "memory") that is auto-injected into
every mote prime output. Memories are flat key/body pairs stored at
.memory/memory.json — they do NOT participate in the mote graph (no
scoring, no edges, no contradiction detection). For richer knowledge,
use 'mote add --type=lesson'.

The key is auto-derived from the body unless --key is given. On
collision, an auto-derived key gets a -2, -3, ... suffix. Explicit
--key collisions overwrite by default; --no-clobber rejects them.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemember,
}

var (
	rememberKey       string
	rememberForce     bool
	rememberNoClobber bool
	rememberQuiet     bool
	rememberJSON      bool
)

func init() {
	rememberCmd.Flags().StringVar(&rememberKey, "key", "", "Explicit key (default: auto-derived from body)")
	rememberCmd.Flags().BoolVar(&rememberForce, "force", false, "Bypass security scan blocks")
	rememberCmd.Flags().BoolVar(&rememberNoClobber, "no-clobber", false, "Fail with non-zero exit if the key already exists")
	rememberCmd.Flags().BoolVar(&rememberQuiet, "quiet", false, "Suppress security scan warnings on stderr")
	rememberCmd.Flags().BoolVar(&rememberJSON, "json", false, "Emit JSON describing the saved memory")
	rootCmd.AddCommand(rememberCmd)
}

func runRemember(cmd *cobra.Command, args []string) error {
	body := args[0]
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("memory body cannot be empty")
	}

	// Security scan parity with `mote add --body`. We do this BEFORE writing
	// because memories auto-inject into agent context via mote prime — a
	// poisoned memory is a stored prompt injection.
	scan := security.ScanBodyContent(body)
	auditor := (*core.AuditLogger)(nil) // lazy init only if we need to log overrides

	if scan.HasBlocks() && !rememberForce {
		return fmt.Errorf("body contains blocked content: %s (use --force to override)",
			strings.Join(scan.BlockedPatterns, ", "))
	}

	root, err := findOrInitMemoryRoot()
	if err != nil {
		return err
	}

	if scan.HasBlocks() && rememberForce {
		auditor = core.NewAuditLogger(root)
		_ = auditor.Log(core.AuditEntry{
			Operation: "security_override",
			MoteID:    "memory:" + rememberKey,
			FieldsSet: scan.BlockedPatterns,
		})
	}
	if scan.HasWarnings() {
		if auditor == nil {
			auditor = core.NewAuditLogger(root)
		}
		_ = auditor.Log(core.AuditEntry{
			Operation: "security_warning",
			MoteID:    "memory:" + rememberKey,
			FieldsSet: scan.Warnings,
		})
		if !rememberQuiet {
			for _, w := range scan.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
		}
	}

	store := core.NewMemoryStore(root)
	actor := core.ResolveAgentID()

	var rec *core.MemoryRecord
	if rememberKey != "" {
		rec, err = store.Put(rememberKey, body, actor, core.PutOpts{NoClobber: rememberNoClobber})
	} else {
		rec, err = store.PutAutoKey(body, actor)
	}
	if err != nil {
		if errors.Is(err, core.ErrMemoryExists) {
			return fmt.Errorf("memory %q already exists (use without --no-clobber to overwrite)", rememberKey)
		}
		return fmt.Errorf("save memory: %w", err)
	}

	if rememberJSON {
		return emitRememberJSON(rec)
	}
	fmt.Printf("Saved memory %s\n", rec.Key)
	return nil
}

func emitRememberJSON(rec *core.MemoryRecord) error {
	payload := struct {
		Key       string `json:"key"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}{
		Key:       rec.Key,
		CreatedAt: rec.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: rec.UpdatedAt.UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal remember response: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// findOrInitMemoryRoot walks for an existing .memory/ dir, falling back
// to <cwd>/.memory for the first remember in a fresh repo. Mirrors the
// pattern in cmd_add.go so `mote remember` works the same way before
// `mote init`.
func findOrInitMemoryRoot() (string, error) {
	root, err := findMemoryRoot()
	if err != nil {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", cwdErr
		}
		root = cwd + string(os.PathSeparator) + ".memory"
	}
	if err := initMemoryDir(root); err != nil {
		return "", fmt.Errorf("init memory dir: %w", err)
	}
	return root, nil
}
