// SPDX-License-Identifier: MIT

// Package doctor implements `mote doctor` checks that operate on repository
// files rather than the mote graph. The first such check is instruction-doc
// reconciliation (STORY-DIVRG-001), which compares designated H2 sections
// across CLAUDE.md, AGENTS.md, CODEX.md, and GEMINI.md.
package doctor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"motes/internal/core"
)

// DivergenceMarker is the HTML comment that, when present in a shared
// section, suppresses drift reporting for that section in that file.
const DivergenceMarker = "<!-- mote-doctor-divergence: ok -->"

// ErrSkipped signals that the instruction-doc check did not run because no
// shared sections are configured. Callers should treat this as a benign
// non-error.
var ErrSkipped = errors.New("instruction-doc check skipped: no shared_sections configured")

// DriftError is returned when a peer file's shared section differs from
// the authoritative file's content (and no divergence-ok marker is present).
type DriftError struct {
	Section           string
	AuthoritativeFile string
	DivergedFiles     []string
}

func (e *DriftError) Error() string {
	return fmt.Sprintf(
		"instruction-doc drift detected:\n  section: %q\n  authoritative file: %s\n  diverged file(s): %s\n  remedy: copy the section verbatim, or add\n          %s\n          immediately under the section heading in the diverged file",
		e.Section, e.AuthoritativeFile, strings.Join(e.DivergedFiles, ", "), DivergenceMarker,
	)
}

// MissingSectionError is returned when a shared section exists in the
// authoritative file but is absent from one or more peer files.
type MissingSectionError struct {
	Section           string
	AuthoritativeFile string
	MissingFrom       []string
}

func (e *MissingSectionError) Error() string {
	return fmt.Sprintf(
		"instruction-doc drift detected:\n  section: %q\n  present in: %s\n  missing from: %s",
		e.Section, e.AuthoritativeFile, strings.Join(e.MissingFrom, ", "),
	)
}

// CheckInstructionDocs compares the H2 sections declared in cfg.SharedSections
// across cfg.ComparePeers (defaulting to the four canonical instruction docs)
// using cfg.AuthoritativeFile (defaulting to CLAUDE.md) as the source of truth.
//
// Returns ErrSkipped when SharedSections is empty. Returns a *DriftError or
// *MissingSectionError on first finding; when multiple sections fail, returns
// errors.Join'd errors. The verbose slice contains per-file status lines for
// `mote doctor --verbose` output.
func CheckInstructionDocs(root string, cfg core.InstructionDocsConfig) ([]string, error) {
	if len(cfg.SharedSections) == 0 {
		return nil, ErrSkipped
	}

	auth := authoritativeFile(cfg)
	peerList := peerFiles(cfg)
	bodies, err := readBodies(root, peerList)
	if err != nil {
		return nil, err
	}
	if bodies[auth] == nil {
		return nil, fmt.Errorf("authoritative file %s is missing or unreadable", auth)
	}

	geminiImports := false
	if g := bodies["GEMINI.md"]; g != nil {
		geminiImports = hasAgentsImport(g)
	}

	var verbose []string
	var findings []error

	for _, heading := range cfg.SharedSections {
		authSection, authOK := extractSection(bodies[auth], heading)
		if !authOK {
			findings = append(findings, fmt.Errorf("authoritative file %s has no section %q", auth, heading))
			continue
		}
		authBody := stripHeading(authSection, heading)

		var diverged []string
		var missing []string

		for _, f := range peerList {
			if f == auth {
				verbose = append(verbose, fmt.Sprintf("%s %s: authoritative", f, heading))
				continue
			}
			body := bodies[f]
			if body == nil {
				// File doesn't exist on disk. Treat as missing.
				missing = append(missing, f)
				verbose = append(verbose, fmt.Sprintf("%s %s: file absent", f, heading))
				continue
			}
			sec, present := extractSection(body, heading)
			if !present {
				if f == "GEMINI.md" && geminiImports {
					if _, agentsOK := extractSection(bodies["AGENTS.md"], heading); agentsOK {
						verbose = append(verbose, fmt.Sprintf("%s %s: imported via @AGENTS.md", f, heading))
						continue
					}
				}
				missing = append(missing, f)
				verbose = append(verbose, fmt.Sprintf("%s %s: missing", f, heading))
				continue
			}
			if hasDivergenceMarker(sec) {
				verbose = append(verbose, fmt.Sprintf("%s %s: divergence-ok (marker present)", f, heading))
				continue
			}
			secBody := stripHeading(sec, heading)
			if normalize(secBody) == normalize(authBody) {
				verbose = append(verbose, fmt.Sprintf("%s %s: matches authoritative", f, heading))
				continue
			}
			diverged = append(diverged, f)
			verbose = append(verbose, fmt.Sprintf("%s %s: diverged", f, heading))
		}

		if len(diverged) > 0 {
			findings = append(findings, &DriftError{
				Section:           heading,
				AuthoritativeFile: auth,
				DivergedFiles:     diverged,
			})
		}
		if len(missing) > 0 {
			findings = append(findings, &MissingSectionError{
				Section:           heading,
				AuthoritativeFile: auth,
				MissingFrom:       missing,
			})
		}
	}

	switch len(findings) {
	case 0:
		return verbose, nil
	case 1:
		return verbose, findings[0]
	default:
		return verbose, errors.Join(findings...)
	}
}

// FixInstructionDocs rewrites diverged sections and appends missing sections
// in peer files to match the authoritative file. Sections carrying the
// DivergenceMarker are skipped. GEMINI.md missing a section that AGENTS.md
// provides (via @AGENTS.md import) is also skipped. Returns verbose lines
// describing every action taken (or skipped).
func FixInstructionDocs(root string, cfg core.InstructionDocsConfig) ([]string, error) {
	if len(cfg.SharedSections) == 0 {
		return nil, ErrSkipped
	}

	auth := authoritativeFile(cfg)
	peerList := peerFiles(cfg)
	bodies, err := readBodies(root, peerList)
	if err != nil {
		return nil, err
	}
	if bodies[auth] == nil {
		return nil, fmt.Errorf("authoritative file %s is missing or unreadable", auth)
	}

	geminiImports := false
	if g := bodies["GEMINI.md"]; g != nil {
		geminiImports = hasAgentsImport(g)
	}

	// Working copies of each peer file; mutated in place across sections so
	// that multiple shared sections within the same file accumulate.
	working := make(map[string][]byte, len(peerList))
	for _, f := range peerList {
		if bodies[f] != nil {
			working[f] = append([]byte(nil), bodies[f]...)
		}
	}

	var verbose []string

	for _, heading := range cfg.SharedSections {
		authSection, authOK := extractSection(bodies[auth], heading)
		if !authOK {
			verbose = append(verbose, fmt.Sprintf("%s %s: authoritative absent — skipped", auth, heading))
			continue
		}

		for _, f := range peerList {
			if f == auth {
				continue
			}
			body, ok := working[f]
			if !ok {
				verbose = append(verbose, fmt.Sprintf("%s %s: file absent — skipped", f, heading))
				continue
			}
			sec, present := extractSection(body, heading)
			if present {
				if hasDivergenceMarker(sec) {
					verbose = append(verbose, fmt.Sprintf("%s %s: skipped (marker present)", f, heading))
					continue
				}
				if normalize(stripHeading(sec, heading)) == normalize(stripHeading(authSection, heading)) {
					continue // already in agreement
				}
				start, end, _ := findSectionRange(body, heading)
				replacement := authSection
				if end < len(body) && !strings.HasSuffix(replacement, "\n") {
					replacement += "\n"
				}
				newBody := make([]byte, 0, len(body)+len(replacement))
				newBody = append(newBody, body[:start]...)
				newBody = append(newBody, replacement...)
				newBody = append(newBody, body[end:]...)
				working[f] = newBody
				verbose = append(verbose, fmt.Sprintf("%s %s: rewrote %d bytes from %s", f, heading, len(replacement), auth))
				continue
			}
			// Section missing.
			if f == "GEMINI.md" && geminiImports {
				if _, agentsOK := extractSection(bodies["AGENTS.md"], heading); agentsOK {
					verbose = append(verbose, fmt.Sprintf("%s %s: imported via @AGENTS.md — skipped", f, heading))
					continue
				}
			}
			// Append with a single blank-line separator.
			trimmed := bytes.TrimRight(body, "\n")
			appendBlock := append([]byte(nil), trimmed...)
			appendBlock = append(appendBlock, '\n', '\n')
			appendBlock = append(appendBlock, []byte(authSection)...)
			if !bytes.HasSuffix(appendBlock, []byte("\n")) {
				appendBlock = append(appendBlock, '\n')
			}
			working[f] = appendBlock
			verbose = append(verbose, fmt.Sprintf("%s %s: appended %d bytes from %s", f, heading, len(authSection), auth))
		}
	}

	for _, f := range peerList {
		original, exists := bodies[f]
		if !exists || original == nil {
			continue
		}
		updated, ok := working[f]
		if !ok {
			continue
		}
		if bytes.Equal(original, updated) {
			continue
		}
		path := filepath.Join(root, f)
		if err := core.AtomicWrite(path, updated, 0o644); err != nil {
			return verbose, fmt.Errorf("write %s: %w", f, err)
		}
	}

	return verbose, nil
}

// --- internal helpers ---

func authoritativeFile(cfg core.InstructionDocsConfig) string {
	if cfg.AuthoritativeFile != "" {
		return cfg.AuthoritativeFile
	}
	return "CLAUDE.md"
}

func peerFiles(cfg core.InstructionDocsConfig) []string {
	if len(cfg.ComparePeers) > 0 {
		return cfg.ComparePeers
	}
	return []string{"CLAUDE.md", "AGENTS.md", "CODEX.md", "GEMINI.md"}
}

func readBodies(root string, peers []string) (map[string][]byte, error) {
	out := make(map[string][]byte, len(peers))
	for _, f := range peers {
		path := filepath.Join(root, f)
		b, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				out[f] = nil
				continue
			}
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		out[f] = b
	}
	return out, nil
}

// findSectionRange locates an H2 section by heading and returns the byte
// range [start, end) within body. start is the offset of the heading line.
// end is the offset of the next H2 heading line, or len(body) if no next
// heading exists.
func findSectionRange(body []byte, heading string) (int, int, bool) {
	src := string(body)
	pos := 0
	headingStart := -1
	for pos <= len(src) {
		nl := strings.IndexByte(src[pos:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(src)
		} else {
			lineEnd = pos + nl
		}
		line := src[pos:lineEnd]
		if headingStart == -1 {
			if strings.TrimRight(line, " \t\r") == heading {
				headingStart = pos
			}
		} else if strings.HasPrefix(line, "## ") {
			return headingStart, pos, true
		}
		if nl < 0 {
			break
		}
		pos = lineEnd + 1
	}
	if headingStart >= 0 {
		return headingStart, len(src), true
	}
	return 0, 0, false
}

// extractSection returns the heading line plus its body (up to but not
// including the next H2). Returns ("", false) when the heading is absent.
func extractSection(body []byte, heading string) (string, bool) {
	start, end, ok := findSectionRange(body, heading)
	if !ok {
		return "", false
	}
	return string(body[start:end]), true
}

// stripHeading removes the heading line and the newline that follows it.
func stripHeading(section, heading string) string {
	s := strings.TrimPrefix(section, heading)
	s = strings.TrimPrefix(s, "\n")
	return s
}

// normalize strips trailing whitespace/newlines so that "foo\n" and "foo"
// compare equal. Internal whitespace is preserved exactly.
func normalize(s string) string {
	return strings.TrimRight(s, "\n \t\r")
}

func hasDivergenceMarker(section string) bool {
	return strings.Contains(section, DivergenceMarker)
}

// hasAgentsImport reports whether body contains a line that is exactly
// "@AGENTS.md" (whitespace-tolerant). Gemini CLI uses this syntax to inline
// AGENTS.md at agent-load time.
func hasAgentsImport(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "@AGENTS.md" {
			return true
		}
	}
	return false
}
