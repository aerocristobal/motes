package security

import (
	"testing"
)

func FuzzValidateMoteID(f *testing.F) {
	f.Add("")
	f.Add("motes-T1234")
	f.Add("../..")
	f.Add("foo/bar")
	f.Add("foo\\bar")
	f.Add("ab\x00cd")
	f.Add("\r\n")
	f.Add(string(make([]byte, 300)))

	f.Fuzz(func(t *testing.T, s string) {
		ValidateMoteID(s) // must not panic
	})
}

func FuzzValidateCorpusName(f *testing.F) {
	f.Add("")
	f.Add("docs")
	f.Add("../..")
	f.Add("CON")
	f.Add("NUL")
	f.Add("a\x00b")
	f.Add("a/b")
	f.Add(string(make([]byte, 200)))

	f.Fuzz(func(t *testing.T, s string) {
		ValidateCorpusName(s) // must not panic
	})
}

func FuzzValidateTag(f *testing.F) {
	f.Add("")
	f.Add("go")
	f.Add("tag with spaces")
	f.Add("tag@invalid")
	f.Add("\xff\xfe")
	f.Add(string(make([]byte, 200)))

	f.Fuzz(func(t *testing.T, s string) {
		ValidateTag(s) // must not panic
	})
}

func FuzzValidateCommand(f *testing.F) {
	f.Add("")
	f.Add("vi")
	f.Add("vi; rm -rf /")
	f.Add("$(rm -rf /)")
	f.Add("vi|cat")
	f.Add("vi && bad")

	f.Fuzz(func(t *testing.T, s string) {
		ValidateCommand(s) // must not panic
	})
}

func FuzzScanBodyContent(f *testing.F) {
	f.Add("")
	f.Add("normal text")
	f.Add("AKIAIOSFODNN7EXAMPLE")
	f.Add("sk_" + "live_TESTDONOTUSE000000000000")
	f.Add("ghp_abcdefghijklmnopqrstuvwxyz0123456789")
	f.Add("-----BEGIN RSA PRIVATE KEY-----")
	f.Add("sk-ant-api03-abcdefghijklmnopqrstuvwxyz")
	f.Add("token = 'secret123'")
	f.Add("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xt")

	f.Fuzz(func(t *testing.T, s string) {
		ScanBodyContent(s) // must not panic
	})
}
