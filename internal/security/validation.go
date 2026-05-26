// SPDX-License-Identifier: MIT
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ValidateCommand checks if a command is safe to execute.
func ValidateCommand(command string) error {
	// Empty command
	if command == "" {
		return fmt.Errorf("empty command")
	}

	// Check for command injection patterns
	dangerousPatterns := []string{
		";", "|", "&", "$", "`", "$(", "${", "||", "&&",
		"<", ">", "\n", "\r", "\t",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("command contains dangerous pattern: %s", pattern)
		}
	}

	// Validate that the command is a simple executable name or absolute path
	if strings.Contains(command, "/") {
		// If it contains a slash, must be an absolute path to an executable
		if !filepath.IsAbs(command) {
			return fmt.Errorf("relative paths not allowed in commands")
		}
		// Check if the file exists and is executable
		info, err := os.Stat(command)
		if err != nil {
			return fmt.Errorf("command file not found: %w", err)
		}
		if info.Mode()&0111 == 0 {
			return fmt.Errorf("command file is not executable")
		}
	} else {
		// Simple command name - validate it's alphanumeric with basic chars
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, command); !matched {
			return fmt.Errorf("invalid command name format")
		}
	}

	return nil
}

// ValidateMoteID checks if a mote ID is safe for file operations.
func ValidateMoteID(id string) error {
	if id == "" {
		return fmt.Errorf("empty mote ID")
	}

	// Check length bounds
	if len(id) > 255 {
		return fmt.Errorf("mote ID too long (max 255 chars)")
	}

	// Check for path traversal attempts
	if strings.Contains(id, "..") {
		return fmt.Errorf("mote ID contains path traversal sequence")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("mote ID contains path separators")
	}

	// Check for dangerous characters
	if strings.ContainsAny(id, "\x00\r\n\t") {
		return fmt.Errorf("mote ID contains null or control characters")
	}

	// Validate expected mote ID format: scope-typechar+base36+random
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, id); !matched {
		return fmt.Errorf("mote ID contains invalid characters")
	}

	return nil
}

// ValidateCorpusName checks if a corpus name is safe for file operations.
func ValidateCorpusName(name string) error {
	if name == "" {
		return fmt.Errorf("empty corpus name")
	}

	// Check length bounds
	if len(name) > 100 {
		return fmt.Errorf("corpus name too long (max 100 chars)")
	}

	// Check for path traversal attempts
	if strings.Contains(name, "..") {
		return fmt.Errorf("corpus name contains path traversal sequence")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("corpus name contains path separators")
	}

	// Check for dangerous characters and reserved names
	if strings.ContainsAny(name, "\x00\r\n\t") {
		return fmt.Errorf("corpus name contains null or control characters")
	}

	// Reserved names
	reserved := []string{".", "..", "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	upperName := strings.ToUpper(name)
	for _, res := range reserved {
		if upperName == res {
			return fmt.Errorf("corpus name is reserved: %s", name)
		}
	}

	// Validate format (alphanumeric, underscore, hyphen, period)
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, name); !matched {
		return fmt.Errorf("corpus name contains invalid characters")
	}

	return nil
}

// ValidateTag checks if a tag name is safe.
func ValidateTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("empty tag")
	}

	if len(tag) > 100 {
		return fmt.Errorf("tag too long (max 100 chars)")
	}

	if !utf8.ValidString(tag) {
		return fmt.Errorf("tag contains invalid UTF-8")
	}

	// Tags should be simple alphanumeric with basic punctuation
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, tag); !matched {
		return fmt.Errorf("tag contains invalid characters")
	}

	return nil
}

// ValidateAgentID checks if a MOTE_AGENT_ID value is safe to use as an audit
// identifier (e.g. when stamping the assignee of an atomically claimed mote).
// Tighter than ValidateTag — it also rejects path separators and Unicode bidi
// control characters that could spoof identity in audit logs.
func ValidateAgentID(id string) error {
	if id == "" {
		return fmt.Errorf("empty agent ID")
	}

	if len(id) > 100 {
		return fmt.Errorf("agent ID too long (max 100 chars)")
	}

	if !utf8.ValidString(id) {
		return fmt.Errorf("agent ID contains invalid UTF-8")
	}

	if strings.Contains(id, "..") {
		return fmt.Errorf("agent ID contains path traversal sequence")
	}

	if strings.ContainsAny(id, "\x00\r\n\t/\\") {
		return fmt.Errorf("agent ID contains null, control, or path characters")
	}

	for _, r := range id {
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return fmt.Errorf("agent ID contains Unicode bidi control characters")
		}
	}

	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, id); !matched {
		return fmt.Errorf("agent ID contains invalid characters")
	}

	return nil
}

// ValidateWeight checks if a weight value is in valid range.
func ValidateWeight(weight float64) error {
	if weight < 0.0 || weight > 1.0 {
		return fmt.Errorf("weight must be between 0.0 and 1.0")
	}
	return nil
}

// ValidateEnum checks if a value is in the allowed enum values.
func ValidateEnum(value string, allowedValues []string, fieldName string) error {
	if value == "" {
		return fmt.Errorf("empty %s", fieldName)
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}

	return fmt.Errorf("invalid %s: %s (allowed: %v)", fieldName, value, allowedValues)
}

// ValidExecutionModes is the canonical allowlist for execution_mode.
// Declared here (rather than imported from core) to avoid an import cycle.
var ValidExecutionModes = []string{"local", "delegated", "parallel"}

// ValidExecutionReasoningEfforts is the canonical allowlist for execution_reasoning_effort.
var ValidExecutionReasoningEfforts = []string{"low", "medium", "high"}

// executionFieldNames lists the five execution_* frontmatter keys that motes
// expose for orchestration hints (STORY-EXEC-001).
var executionFieldNames = map[string]bool{
	"execution_agent_type":       true,
	"execution_suggested_model":  true,
	"execution_reasoning_effort": true,
	"execution_mode":             true,
	"execution_parallel_group":   true,
}

// ValidateExecutionField validates a single execution_* field by name.
// Empty values are accepted (they signal "clear this field"); callers that
// require a non-empty value should check separately.
//
// Enum fields delegate to ValidateEnum. Free-form fields enforce:
//   - max 256 chars
//   - UTF-8 valid
//   - no path traversal, path separators, or null/control characters
//   - no shell metacharacters ($, `, ;, |, &, etc.)
//   - no Unicode bidi-override characters (audit-log spoofing defence)
//   - alphanumeric plus . _ -
func ValidateExecutionField(name, value string) error {
	if !executionFieldNames[name] {
		return fmt.Errorf("unknown execution field: %s", name)
	}

	// Empty = clear the field. Allowed for all execution fields.
	if value == "" {
		return nil
	}

	switch name {
	case "execution_mode":
		if err := ValidateEnum(value, ValidExecutionModes, "execution_mode"); err != nil {
			return err
		}
		return nil
	case "execution_reasoning_effort":
		if err := ValidateEnum(value, ValidExecutionReasoningEfforts, "execution_reasoning_effort"); err != nil {
			return err
		}
		return nil
	default:
		return validateExecutionFreeForm(name, value)
	}
}

// validateExecutionFreeForm enforces the free-form ruleset for
// execution_agent_type, execution_suggested_model, and execution_parallel_group.
func validateExecutionFreeForm(name, value string) error {
	if len(value) > 256 {
		return fmt.Errorf("invalid %s: %s too long (max 256 chars)", name, name)
	}

	if !utf8.ValidString(value) {
		return fmt.Errorf("invalid %s: contains invalid UTF-8", name)
	}

	if strings.Contains(value, "..") {
		return fmt.Errorf("invalid %s: contains path traversal sequence", name)
	}

	if strings.ContainsAny(value, "\x00\r\n\t/\\") {
		return fmt.Errorf("invalid %s: contains invalid character (null, control, or path separator)", name)
	}

	for _, r := range value {
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return fmt.Errorf("invalid %s: contains invalid character (Unicode bidi control)", name)
		}
	}

	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9._-]+$`, value); !matched {
		return fmt.Errorf("invalid %s: contains invalid characters (allowed: alphanumeric . _ -)", name)
	}

	return nil
}

// MetadataKeyMaxLen caps `--metadata-field key` and `--has-metadata-key key`
// inputs at the read-time query surface. Per STORY-MQRY-001 §4 Q1.
const MetadataKeyMaxLen = 256

// MetadataValueMaxLen caps `--metadata-field key=value` values. Larger than
// the 256-char cap on execution_* writes because the filter is schema-agnostic
// and may match future fields that legitimately hold longer values.
const MetadataValueMaxLen = 4096

var metadataKeyRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ValidateMetadataKey enforces the read-time validation surface for metadata
// query keys. Keys must be alphanumeric + underscore only — no dots (no nested
// traversal), no slashes (no path traversal), no shell metacharacters, no
// whitespace, no Unicode bidi controls, no NUL byte. Empty rejected.
//
// Per STORY-MQRY-001 §4 Q1 (validator location) and §4 Q3 (any well-formed
// key accepted; schema validity is decided by "returns empty result for
// unknown keys").
func ValidateMetadataKey(key string) error {
	if key == "" {
		return fmt.Errorf("invalid metadata key: empty")
	}

	if len(key) > MetadataKeyMaxLen {
		return fmt.Errorf("metadata key too long (max %d chars)", MetadataKeyMaxLen)
	}

	if !utf8.ValidString(key) {
		return fmt.Errorf("invalid metadata key: contains invalid UTF-8")
	}

	for _, r := range key {
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return fmt.Errorf("invalid metadata key: contains Unicode bidi control characters")
		}
	}

	if !metadataKeyRe.MatchString(key) {
		return fmt.Errorf("invalid metadata key: only [a-zA-Z0-9_] allowed")
	}

	return nil
}

// ValidateMetadataValue enforces shape constraints on `--metadata-field
// key=value` values. The filter operates against already-loaded motes — no
// SQL or shell interpolation — so this is purely a defence-in-depth check
// against pathologically large or audit-log-spoofing inputs. Empty allowed
// (it matches motes with the key explicitly set to "").
//
// Per STORY-MQRY-001 §4 Q2 (4096-char cap) and the "Adversarial values"
// scenario outline (bidi, NUL byte, length).
func ValidateMetadataValue(value string) error {
	if len(value) > MetadataValueMaxLen {
		return fmt.Errorf("invalid metadata value: too long (max %d chars)", MetadataValueMaxLen)
	}

	if !utf8.ValidString(value) {
		return fmt.Errorf("invalid metadata value: contains invalid UTF-8")
	}

	for _, r := range value {
		if r == 0 {
			return fmt.Errorf("invalid metadata value: contains NUL byte")
		}
		if (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) {
			return fmt.Errorf("invalid metadata value: contains Unicode bidi control characters")
		}
	}

	return nil
}

// ValidateBodySize checks if body content is within reasonable size limits.
func ValidateBodySize(body string) error {
	const maxBodySize = 1 * 1024 * 1024 // 1MB

	if len(body) > maxBodySize {
		return fmt.Errorf("body content too large (max 1MB)")
	}

	if !utf8.ValidString(body) {
		return fmt.Errorf("body contains invalid UTF-8")
	}

	return nil
}

// ScanResult holds the outcome of body content secret scanning.
type ScanResult struct {
	BlockedPatterns []string // high-confidence matches (hard block)
	Warnings        []string // ambiguous matches (stderr warning only)
}

// HasBlocks returns true if any high-confidence secret patterns were detected.
func (r ScanResult) HasBlocks() bool { return len(r.BlockedPatterns) > 0 }

// HasWarnings returns true if any ambiguous secret-like patterns were detected.
func (r ScanResult) HasWarnings() bool { return len(r.Warnings) > 0 }

// secretPattern defines a compiled regex with its human-readable description.
type secretPattern struct {
	re   *regexp.Regexp
	desc string
}

// High-confidence patterns that block mote creation/update.
var blockPatterns = []secretPattern{
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "potential AWS access key detected in body"},
	{regexp.MustCompile(`sk_live_[0-9a-zA-Z]{24,}`), "potential Stripe secret key detected in body"},
	{regexp.MustCompile(`ghp_[0-9a-zA-Z]{36}`), "potential GitHub personal access token detected in body"},
	{regexp.MustCompile(`github_pat_[0-9a-zA-Z_]{22,}`), "potential GitHub fine-grained token detected in body"},
	{regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`), "potential private key material detected in body"},
	{regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]{20,}`), "potential Anthropic API key detected in body"},
	{regexp.MustCompile(`(?i)(?:cloudflare|cf_api|x-auth-key)\S*\s*[:=]\s*["']?[0-9a-f]{37}`), "potential Cloudflare API key detected in body"},
	{regexp.MustCompile(`(?i)(?:cloudflare|cf_api_token|cf_bearer)\S*\s*[:=]\s*["']?[A-Za-z0-9_-]{40,}`), "potential Cloudflare API token detected in body"},
}

// Ambiguous patterns that produce warnings but don't block.
var warnPatterns = []secretPattern{
	{regexp.MustCompile(`(?i)(?:token|secret|password|api_key)\s*[:=]\s*["']?[A-Za-z0-9/+=]{8,}`), "potential credential assignment detected in body"},
	{regexp.MustCompile(`(?:^|\s|["'])([A-Za-z0-9+/]{40,}={0,2})(?:\s|["']|$)`), "potential base64-encoded secret detected in body"},
}

// ScanBodyContent checks body text for secret patterns.
// It returns both blocking matches and non-blocking warnings.
func ScanBodyContent(body string) ScanResult {
	var result ScanResult

	for _, p := range blockPatterns {
		if p.re.MatchString(body) {
			result.BlockedPatterns = append(result.BlockedPatterns, p.desc)
		}
	}

	for _, p := range warnPatterns {
		if p.re.MatchString(body) {
			result.Warnings = append(result.Warnings, p.desc)
		}
	}

	return result
}

// SafeBounds checks if an index is safe for slice/string access.
func SafeBounds(index, length int) error {
	if index < 0 {
		return fmt.Errorf("negative index: %d", index)
	}
	if index >= length {
		return fmt.Errorf("index %d out of bounds for length %d", index, length)
	}
	return nil
}

// SafeSubstring safely extracts a substring with bounds checking.
func SafeSubstring(s string, start, end int) (string, error) {
	if start < 0 {
		return "", fmt.Errorf("negative start index: %d", start)
	}
	if end < start {
		return "", fmt.Errorf("end index %d less than start index %d", end, start)
	}
	if start > len(s) {
		return "", fmt.Errorf("start index %d out of bounds for string length %d", start, len(s))
	}
	if end > len(s) {
		return "", fmt.Errorf("end index %d out of bounds for string length %d", end, len(s))
	}
	return s[start:end], nil
}
