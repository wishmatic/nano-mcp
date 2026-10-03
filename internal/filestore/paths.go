package filestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (c *Client) safePath(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("filestore: empty key")
	}

	if strings.HasPrefix(key, "/") || filepath.IsAbs(key) {
		return "", fmt.Errorf("filestore: key %q is absolute", key)
	}

	segments := strings.Split(key, "/")
	if segments[0] != namespace {
		return "", fmt.Errorf("filestore: key %q is outside the %s namespace", key, namespace)
	}

	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("filestore: key %q has an invalid segment", key)
		}
	}

	full := filepath.Join(c.cfg.Dir, filepath.FromSlash(key))

	rel, err := filepath.Rel(c.cfg.Dir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("filestore: key %q escapes the storage directory", key)
	}

	return full, nil
}

// writeFileAtomic keeps a reader from seeing a half-written video, and keeps a failed download
// from leaving a truncated file behind.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return err
	}

	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()

		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	tmpName = ""

	return nil
}
