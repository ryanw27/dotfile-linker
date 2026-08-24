// Package dotlink links files from a dotfiles repository into a target
// directory (normally $HOME) as symlinks named with a leading dot.
//
// A file named "bashrc" in the source directory maps to "~/.bashrc" in
// the target. Nothing recursive, nothing clever about naming: the point
// is to keep the mapping obvious from looking at the repo.
package dotlink

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ActionKind describes what Plan decided needs to happen to a single file.
type ActionKind int

const (
	// Create means the target does not exist yet.
	Create ActionKind = iota
	// Relink means the target can be safely replaced: either it's a
	// symlink pointing somewhere else, or it's a regular file whose
	// content is identical to the source.
	Relink
	// Conflict means the target is a regular file with content that
	// differs from the source. Apply refuses to touch it.
	Conflict
	// NoOp means the target already links to the source.
	NoOp
)

func (k ActionKind) String() string {
	switch k {
	case Create:
		return "create"
	case Relink:
		return "relink"
	case Conflict:
		return "conflict"
	case NoOp:
		return "up to date"
	default:
		return "unknown"
	}
}

// Action is one file's worth of plan: what needs to happen to bring
// Target in sync with Source.
type Action struct {
	Name   string // file name as it appears in the source dir, e.g. "bashrc"
	Source string // absolute path to the file in the dotfiles repo
	Target string // absolute path to the symlink location, e.g. ~/.bashrc
	Kind   ActionKind
}

// Plan compares every top-level file in sourceDir against its expected
// symlink in targetDir and returns the action needed for each one. It
// only reads the filesystem; it never modifies it.
func Plan(sourceDir, targetDir string) ([]Action, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("reading source dir: %w", err)
	}

	var actions []Action
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}

		source, err := filepath.Abs(filepath.Join(sourceDir, name))
		if err != nil {
			return nil, err
		}
		target := filepath.Join(targetDir, "."+name)

		kind, err := classify(source, target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		actions = append(actions, Action{Name: name, Source: source, Target: target, Kind: kind})
	}
	return actions, nil
}

func classify(source, target string) (ActionKind, error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return Create, nil
	}
	if err != nil {
		return 0, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(target)
		if err != nil {
			return 0, err
		}
		if dest == source {
			return NoOp, nil
		}
		return Relink, nil
	}

	// Target is a regular file. Only offer to replace it automatically
	// if it's byte-for-byte identical to the source; otherwise it might
	// hold local edits we shouldn't silently discard.
	same, err := sameContent(source, target)
	if err != nil {
		return 0, err
	}
	if same {
		return Relink, nil
	}
	return Conflict, nil
}

// Apply performs the filesystem change described by a. Conflict actions
// are refused: the caller has to resolve them (move the file aside,
// diff it by hand, whatever) and rerun Plan before calling Apply again.
func Apply(a Action) error {
	switch a.Kind {
	case NoOp:
		return nil
	case Conflict:
		return fmt.Errorf("%s: refusing to overwrite %s, content differs from %s", a.Name, a.Target, a.Source)
	case Relink:
		if err := os.Remove(a.Target); err != nil {
			return fmt.Errorf("%s: removing existing target: %w", a.Name, err)
		}
		fallthrough
	case Create:
		if err := os.MkdirAll(filepath.Dir(a.Target), 0o755); err != nil {
			return fmt.Errorf("%s: creating parent dir: %w", a.Name, err)
		}
		if err := os.Symlink(a.Source, a.Target); err != nil {
			return fmt.Errorf("%s: creating symlink: %w", a.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("%s: unknown action kind %v", a.Name, a.Kind)
	}
}

func sameContent(a, b string) (bool, error) {
	ha, err := hashFile(a)
	if err != nil {
		return false, err
	}
	hb, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}
