package security

import (
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
	allowedStatuses := []string{"active", "deprecated", "completed", "archived"}

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"active", false},
		{"deprecated", false},
		{"completed", false},
		{"archived", false},
		{"", true},         // empty
		{"invalid", true},  // not in allowed values
		{"ACTIVE", true},   // case sensitive
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
