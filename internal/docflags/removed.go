// SPDX-License-Identifier: MIT
package docflags

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// LoadRemoved reads the removed-flags manifest at path and returns a
// nested map: command-path → set of flag names. Lines starting with
// `#` (after trimming) are comments; blank lines are skipped. Each
// content line has the shape `<command path> <--flag>` where the
// command path may be one or two whitespace-separated words.
//
// A missing manifest is not an error — it returns an empty map. The
// CI bootstrap path lands an empty file, so a fresh checkout still
// runs the check successfully.
func LoadRemoved(path string) (map[string]map[string]bool, error) {
	out := make(map[string]map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("open removed-flags manifest: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s:%d: expected `<command> <--flag>`, got %q",
				path, lineNum, line)
		}

		// The flag is the last field; everything before is the command path.
		flag := fields[len(fields)-1]
		if !strings.HasPrefix(flag, "--") {
			return nil, fmt.Errorf("%s:%d: flag must start with `--`, got %q",
				path, lineNum, flag)
		}
		cmd := strings.Join(fields[:len(fields)-1], " ")
		if _, ok := out[cmd]; !ok {
			out[cmd] = make(map[string]bool)
		}
		out[cmd][flag] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan removed-flags manifest: %w", err)
	}
	return out, nil
}
