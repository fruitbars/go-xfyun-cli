package outputfile

import (
	"fmt"
	"os"
	"path/filepath"
)

func Prepare(path string, force bool) (string, *os.File, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, fmt.Errorf("resolve output path: %w", err)
	}
	if info, err := os.Lstat(absolute); err == nil {
		if info.IsDir() {
			return "", nil, fmt.Errorf("output path is a directory: %s", absolute)
		}
		if !force {
			return "", nil, fmt.Errorf("output already exists; use --force or force=true to replace it: %s", absolute)
		}
	} else if !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("inspect output path: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(absolute), ".xfyun-*.tmp")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary output: %w", err)
	}
	return absolute, temporary, nil
}

// Commit requires a closed temporary file in the destination directory.
func Commit(temporary, destination string, force bool) error {
	if force {
		// Do not delete the old file first: a failed rename must preserve it.
		return os.Rename(temporary, destination)
	}
	// Linking creates the destination atomically and fails if it already exists.
	// Never fall back to rename, which can silently overwrite on Unix.
	if err := os.Link(temporary, destination); err != nil {
		return fmt.Errorf("commit output without overwriting: %w", err)
	}
	_ = os.Remove(temporary)
	return nil
}
