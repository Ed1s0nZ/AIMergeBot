package platform

import (
	"os"
	"path/filepath"
)

// replacementPossible stays true after attempting rename: an ambiguous
// filesystem failure must not reopen the credential gate on the old snapshot.
func persistSettingsFile(path string, raw []byte, syncDirectory bool) (replacementPossible bool, err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".aim-config-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return false, err
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return true, err
	}
	if syncDirectory {
		dir, err := os.Open(filepath.Dir(path))
		if err != nil {
			return true, err
		}
		err = dir.Sync()
		closeErr := dir.Close()
		if err != nil {
			return true, err
		}
		if closeErr != nil {
			return true, closeErr
		}
	}
	return true, nil
}
