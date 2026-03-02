package core

import "os"

// AtomicWrite writes data to path using write-to-temp-then-rename for POSIX atomicity.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
