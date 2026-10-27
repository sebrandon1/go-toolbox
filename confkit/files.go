package confkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to path via a sibling temporary file and rename,
// so a crash mid-write leaves the previous file intact. Parent directories are
// created with 0700. A perm of 0 means 0600.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = 0o600
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename %s: %w", tmpName, err)
	}
	return nil
}

// ValidateJoin joins validation errors into one error, returning nil when there
// are none. It mirrors errors.Join with a name that reads well at call sites.
func ValidateJoin(errs ...error) error {
	return errors.Join(errs...)
}
