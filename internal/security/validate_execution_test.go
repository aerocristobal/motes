// SPDX-License-Identifier: MIT
package security

import (
	"strings"
	"testing"
)

func TestValidateExecutionField_UnknownField(t *testing.T) {
	if err := ValidateExecutionField("execution_unknown", "x"); err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestValidateExecutionField_ModeAccepts(t *testing.T) {
	for _, v := range []string{"local", "delegated", "parallel"} {
		if err := ValidateExecutionField("execution_mode", v); err != nil {
			t.Errorf("execution_mode=%q: unexpected error: %v", v, err)
		}
	}
}

func TestValidateExecutionField_ModeRejectsUnknown(t *testing.T) {
	err := ValidateExecutionField("execution_mode", "fire_and_forget")
	if err == nil {
		t.Fatal("expected rejection")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid execution_mode") {
		t.Errorf("error must contain 'invalid execution_mode': %q", msg)
	}
	for _, v := range []string{"local", "delegated", "parallel"} {
		if !strings.Contains(msg, v) {
			t.Errorf("error must list valid value %q: %q", v, msg)
		}
	}
}

func TestValidateExecutionField_EffortAccepts(t *testing.T) {
	for _, v := range []string{"low", "medium", "high"} {
		if err := ValidateExecutionField("execution_reasoning_effort", v); err != nil {
			t.Errorf("execution_reasoning_effort=%q: unexpected error: %v", v, err)
		}
	}
}

func TestValidateExecutionField_EffortRejectsUnknown(t *testing.T) {
	err := ValidateExecutionField("execution_reasoning_effort", "maximum")
	if err == nil {
		t.Fatal("expected rejection")
	}
	if !strings.Contains(err.Error(), "invalid execution_reasoning_effort") {
		t.Errorf("error must contain 'invalid execution_reasoning_effort': %q", err.Error())
	}
}

func TestValidateExecutionField_EmptyClearsAllFields(t *testing.T) {
	for _, name := range []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode", "execution_parallel_group",
	} {
		if err := ValidateExecutionField(name, ""); err != nil {
			t.Errorf("%s: empty value should be accepted (clear semantics), got %v", name, err)
		}
	}
}

func TestValidateExecutionField_FreeForm_Accepts(t *testing.T) {
	cases := []string{
		"mote-subagent",
		"claude-sonnet-4-7",
		"haiku",
		"group_A",
		"group-A.1",
		"a",
	}
	for _, name := range []string{"execution_agent_type", "execution_suggested_model", "execution_parallel_group"} {
		for _, v := range cases {
			if err := ValidateExecutionField(name, v); err != nil {
				t.Errorf("%s=%q: unexpected error: %v", name, v, err)
			}
		}
	}
}

func TestValidateExecutionField_FreeForm_RejectsAdversarial(t *testing.T) {
	cases := []struct {
		name, label, value string
	}{
		{"execution_agent_type", "dollar-paren", "$(rm -rf ~)"},
		{"execution_agent_type", "backtick", "foo`bar`"},
		{"execution_agent_type", "semicolon", "foo;bar"},
		{"execution_agent_type", "newline", "foo\nbar"},
		{"execution_agent_type", "space", "foo bar"},
		{"execution_agent_type", "traversal", "../../etc/passwd"},
		{"execution_agent_type", "slash", "foo/bar"},
		{"execution_agent_type", "backslash", "foo\\bar"},
		{"execution_agent_type", "tab", "foo\tbar"},
		{"execution_agent_type", "null", "foo\x00bar"},
		{"execution_agent_type", "bidi-rlo", "foo\u202ebar"},
		{"execution_agent_type", "bidi-lro", "foo\u202dbar"},
		{"execution_agent_type", "bidi-isolate", "foo\u2066bar"},
		{"execution_suggested_model", "dollar-paren", "$(curl evil)"},
		{"execution_parallel_group", "pipe", "a|b"},
		{"execution_parallel_group", "ampersand", "a&b"},
	}
	for _, c := range cases {
		err := ValidateExecutionField(c.name, c.value)
		if err == nil {
			t.Errorf("%s/%s=%q: expected rejection, got nil", c.name, c.label, c.value)
			continue
		}
		if !strings.Contains(err.Error(), "invalid "+c.name) {
			t.Errorf("%s/%s: error must contain field name: %q", c.name, c.label, err.Error())
		}
	}
}

func TestValidateExecutionField_FreeForm_LengthBoundary(t *testing.T) {
	at256 := strings.Repeat("a", 256)
	if err := ValidateExecutionField("execution_parallel_group", at256); err != nil {
		t.Errorf("256 chars must be accepted, got %v", err)
	}
	at257 := strings.Repeat("a", 257)
	err := ValidateExecutionField("execution_parallel_group", at257)
	if err == nil {
		t.Fatal("257 chars must be rejected")
	}
	if !strings.Contains(err.Error(), "execution_parallel_group") || !strings.Contains(err.Error(), "too long") {
		t.Errorf("error must mention field and 'too long': %q", err.Error())
	}
}

func FuzzValidateExecutionField(f *testing.F) {
	seeds := []string{"", "haiku", "claude-sonnet-4-7", "local", "high",
		"$(rm)", "foo\u202ebar", strings.Repeat("a", 1000)}
	for _, s := range seeds {
		f.Add(s)
	}
	names := []string{
		"execution_agent_type", "execution_suggested_model",
		"execution_reasoning_effort", "execution_mode",
		"execution_parallel_group", "bogus",
	}
	f.Fuzz(func(t *testing.T, v string) {
		for _, n := range names {
			// Panic-free is the contract; error vs nil is fine either way.
			_ = ValidateExecutionField(n, v)
		}
	})
}
