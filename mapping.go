package dotlink

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadMapping reads a mapping file that overrides the default "name" ->
// ".name" target for individual source files. Each non-blank, non-comment
// line has the form "name target", where target is a path relative to the
// target directory (no leading dot is added, so "config/git/config" ends
// up at "<targetDir>/config/git/config"). Lines starting with '#', and
// blank lines, are ignored.
//
// A missing file is not an error: it just means no overrides are in
// effect, so callers can point at a default path unconditionally.
func LoadMapping(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mapping := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: expected \"name target\", got %q", path, lineNum, line)
		}
		mapping[fields[0]] = fields[1]
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return mapping, nil
}

// targetFor returns the path a source file named name should be linked to
// inside targetDir: the mapping override if one exists, otherwise the
// default of the name with a leading dot.
func targetFor(targetDir, name string, mapping map[string]string) string {
	if rel, ok := mapping[name]; ok {
		return filepath.Join(targetDir, rel)
	}
	return filepath.Join(targetDir, "."+name)
}
