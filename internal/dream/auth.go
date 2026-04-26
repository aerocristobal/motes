// SPDX-License-Identifier: AGPL-3.0-or-later
package dream

import (
	"fmt"
	"os"
)

// resolveAuth returns the credential string for an HTTP-based provider.
//
// The `auth` argument can be:
//   - An environment variable name (recommended). If os.LookupEnv finds it,
//     the variable's value is returned. If the name is set but empty, the
//     empty value is returned as-is — callers should treat that as a config
//     error.
//   - A literal credential string. Used as-is. Convenient for one-off testing
//     but checked-in literals are a security smell; prefer env vars.
//   - The empty string. Always an error — distinguishes "user forgot to
//     configure" from "user set the env var to empty".
//
// The heuristic for env-var-vs-literal is "does os.LookupEnv find this name in
// the environment". Names that look like env vars (UPPERCASE_WITH_UNDERSCORES)
// but aren't set produce a clear error rather than silently being treated as
// literals — which would otherwise send the env var name itself to the API as
// a credential.
func resolveAuth(auth string) (string, error) {
	if auth == "" {
		return "", fmt.Errorf("auth is empty; set provider.auth to an env var name or literal credential")
	}
	if value, found := os.LookupEnv(auth); found {
		return value, nil
	}
	if looksLikeEnvVarName(auth) {
		return "", fmt.Errorf("auth %q looks like an environment variable name but is not set; export it or use a literal value", auth)
	}
	return auth, nil
}

// looksLikeEnvVarName returns true for strings that look like SHELL-style
// environment variable names: uppercase letters, digits, underscores, with at
// least one underscore. Heuristic only — never decisive on its own.
func looksLikeEnvVarName(s string) bool {
	if s == "" {
		return false
	}
	hasUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
			hasUnderscore = true
		default:
			return false
		}
	}
	return hasUnderscore
}
