// SPDX-License-Identifier: AGPL-3.0-or-later
package security

import (
	"strings"
	"testing"
)

func TestValidateCommand(t *testing.T) {
	tests := []struct {
		command string
		wantErr bool
	}{
		{"vi", false},
		{"nano", false},
		{"emacs", false},
		{"/usr/bin/vim", false},
		{"code", false},
		{"", true},                      // empty
		{"vi; rm -rf /", true},          // semicolon injection
		{"vi | cat /etc/passwd", true},  // pipe injection
		{"vi && rm -rf /", true},        // && injection
		{"vi || rm -rf /", true},        // || injection
		{"$(rm -rf /)", true},           // command substitution
		{"vi\nrm -rf /", true},          // newline injection
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			err := ValidateCommand(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommand(%q) error = %v, wantErr %v", tt.command, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMoteID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"motes-T1234567890abcdef", false},
		{"proj-D987654321fedcba", false},
		{"test-L123", false},
		{"", true},                                // empty
		{"motes-T../../../etc/passwd", true},      // path traversal
		{"motes-T123/456", true},                  // forward slash
		{"motes-T123\\456", true},                 // backslash
		{"motes-T123\x00", true},                  // null byte
		{"motes-T123\n", true},                    // newline
		{"motes-T123\t", true},                    // tab
		{string(make([]byte, 300)), true},         // too long
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			err := ValidateMoteID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMoteID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCorpusName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"docs", false},
		{"source-code", false},
		{"my_corpus", false},
		{"corpus.v1", false},
		{"", true},                    // empty
		{"docs/../../../etc", true},   // path traversal
		{"docs/subdir", true},         // forward slash
		{"docs\\subdir", true},        // backslash
		{"CON", true},                 // reserved Windows name
		{"docs\x00", true},            // null byte
		{string(make([]byte, 200)), true}, // too long
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCorpusName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCorpusName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTag(t *testing.T) {
	tests := []struct {
		tag     string
		wantErr bool
	}{
		{"go", false},
		{"security", false},
		{"bug-fix", false},
		{"v1.0", false},
		{"", true},                     // empty
		{string(make([]byte, 200)), true}, // too long
		{"tag with spaces", true},      // invalid chars
		{"tag@invalid", true},          // invalid chars
		{"tag#invalid", true},          // invalid chars
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			err := ValidateTag(tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTag(%q) error = %v, wantErr %v", tt.tag, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWeight(t *testing.T) {
	tests := []struct {
		weight  float64
		wantErr bool
	}{
		{0.0, false},
		{0.5, false},
		{1.0, false},
		{-0.1, true},  // negative
		{1.1, true},   // too large
		{-1.0, true},  // negative
		{2.0, true},   // too large
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := ValidateWeight(tt.weight)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWeight(%f) error = %v, wantErr %v", tt.weight, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEnum(t *testing.T) {
	allowedStatuses := []string{"active", "in_progress", "deprecated", "completed", "archived"}

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"active", false},
		{"in_progress", false},
		{"deprecated", false},
		{"completed", false},
		{"archived", false},
		{"", true},        // empty
		{"invalid", true}, // not in allowed values
		{"ACTIVE", true},  // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := ValidateEnum(tt.value, allowedStatuses, "status")
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnum(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

// testStripeKey builds a test Stripe key from prefix + suffix to avoid
// triggering GitHub push protection on the literal string.
func testStripeKey() string { return "sk_" + "live_TESTDONOTUSE000000000000" }

func TestScanBodyContent(t *testing.T) {
	stripeKey := testStripeKey()

	tests := []struct {
		name       string
		body       string
		wantBlocks int
		wantWarns  int
	}{
		{"clean body", "This is a normal mote about Go concurrency patterns.", 0, 0},
		{"AWS access key", "Found key AKIAIOSFODNN7EXAMPLE in config", 1, 0},
		{"Stripe secret key", "stripe_key: " + stripeKey, 1, 0},
		{"GitHub PAT", "export GH_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789", 1, 0},
		{"GitHub fine-grained", "token: github_pat_1234567890abcdefghijkl", 1, 0},
		{"RSA private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...", 1, 0},
		{"generic private key", "-----BEGIN PRIVATE KEY-----\nMIIE...", 1, 0},
		{"OPENSSH private key", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Blbn...", 1, 0},
		{"Anthropic API key", "Using sk-ant-api03-abcdefghijklmnopqrstuvwxyz for calls", 1, 0},
		{"Cloudflare API key in context", "cf_api_key = \"0123456789abcdef0123456789abcdef01234\"", 1, 1},
		{"Cloudflare token in context", "cloudflare_token: abcdefghijklmnopqrstuvwxyz01234567890123", 1, 2},
		{"bare hex 37 chars no CF context", "0123456789abcdef0123456789abcdef01234", 0, 0},
		{"token assignment warns", "auth_token = \"a1Kx9mP2qR3sT4uV5wX6\"", 0, 1},
		{"password assignment warns", "password: mySecretPass123!", 0, 1},
		{"long base64 warns", "data: \"QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xt\"", 0, 1},
		{"short token no warn", "id: \"abc123\"", 0, 0},
		{"near-miss AKIA too short", "prefix AKIA12345 is not a full key", 0, 0},
		{"sk_test not blocked", "sk_" + "test_abcdef1234567890abcdef12 is a test key", 0, 0},
		{"mixed block and warn", "AKIAIOSFODNN7EXAMPLE\ntoken = \"longSecretValue123\"", 1, 1},
		{"multiple blocks", "AKIAIOSFODNN7EXAMPLE and " + stripeKey, 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ScanBodyContent(tt.body)
			if len(result.BlockedPatterns) != tt.wantBlocks {
				t.Errorf("ScanBodyContent() blocks = %d (%v), want %d", len(result.BlockedPatterns), result.BlockedPatterns, tt.wantBlocks)
			}
			if len(result.Warnings) != tt.wantWarns {
				t.Errorf("ScanBodyContent() warnings = %d (%v), want %d", len(result.Warnings), result.Warnings, tt.wantWarns)
			}
			// Verify blocked descriptions never contain the actual secret
			for _, desc := range result.BlockedPatterns {
				if strings.Contains(desc, "AKIA") && strings.Contains(desc, "IOSFODNN") {
					t.Error("blocked pattern description leaks actual secret value")
				}
				if strings.Contains(desc, "sk_"+"live_") && strings.Contains(desc, "TESTDONOTUSE") {
					t.Error("blocked pattern description leaks actual secret value")
				}
			}
		})
	}
}

func TestScanBodyContent_NoLeakedSecrets(t *testing.T) {
	secrets := []string{
		"AKIAIOSFODNN7EXAMPLE",
		testStripeKey(),
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
	}
	for _, secret := range secrets {
		result := ScanBodyContent("body with " + secret)
		for _, desc := range result.BlockedPatterns {
			if strings.Contains(desc, secret) {
				t.Errorf("description %q contains secret %q", desc, secret)
			}
		}
	}
}

func TestSafeSubstring(t *testing.T) {
	tests := []struct {
		s       string
		start   int
		end     int
		want    string
		wantErr bool
	}{
		{"hello", 0, 5, "hello", false},
		{"hello", 1, 4, "ell", false},
		{"hello", 0, 0, "", false},
		{"hello", -1, 3, "", true},  // negative start
		{"hello", 2, 1, "", true},   // end < start
		{"hello", 0, 10, "", true},  // end > length
		{"hello", 10, 15, "", true}, // start > length
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got, err := SafeSubstring(tt.s, tt.start, tt.end)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeSubstring(%q, %d, %d) error = %v, wantErr %v", tt.s, tt.start, tt.end, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("SafeSubstring(%q, %d, %d) = %q, want %q", tt.s, tt.start, tt.end, got, tt.want)
			}
		})
	}
}
