// SPDX-License-Identifier: MIT

package githooks

import _ "embed"

//go:embed templates/post-checkout
var TemplatePostCheckout []byte

//go:embed templates/pre-commit
var TemplatePreCommit []byte

// Template describes one embedded git-hook script the mote binary ships into
// end-user projects under `.git/hooks/<event>`.
type Template struct {
	// Event is the git hook event name and the basename of the destination
	// file (e.g. "post-checkout").
	Event string
	// Body is the bare embedded script, including the shebang. The install
	// renderer injects the sentinel header lines immediately after the
	// shebang at write time; the embedded body itself does NOT contain
	// `# managed-by:` / `# mote-binary-version:` / `# template-sha256:`.
	Body []byte
}

// Templates returns the full set of git-hook templates this binary ships,
// in the order they should appear in install reports.
func Templates() []Template {
	return []Template{
		{Event: "post-checkout", Body: TemplatePostCheckout},
		{Event: "pre-commit", Body: TemplatePreCommit},
	}
}
