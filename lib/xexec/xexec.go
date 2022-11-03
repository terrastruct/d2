package xexec

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// findExecutable is from package exec
func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return fs.ErrPermission
}

// SearchPath searches for all executables that have prefix in their names in
// the directories named by the PATH environment variable.
func SearchPath(prefix string) ([]string, error) {
	var matches []string
	envPath := os.Getenv("PATH")
	dirSet := make(map[string]struct{})
	for _, dir := range filepath.SplitList(envPath) {
		if dir == "" {
			// From exec package:
			// Unix shell semantics: path element "" means "."
			dir = "."
		}
		if _, ok := dirSet[dir]; ok {
			continue
		}
		dirSet[dir] = struct{}{}
		files, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), prefix) {
				match := filepath.Join(dir, f.Name())
				if err := findExecutable(match); err == nil {
					matches = append(matches, match)
				}
			}
		}

	}
	return matches, nil
}
