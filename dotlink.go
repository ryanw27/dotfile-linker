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
	names, err := sourceFiles(sourceDir)
	if err != nil {
		return nil, err
	}

	var actions []Action
	for _, name := range names {
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

// sourceFiles lists the top-level, non-hidden file names in sourceDir that
// Plan and PlanUnlink both operate on.
func sourceFiles(sourceDir string) ([]string, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("reading source dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
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

// ApplyBackup behaves like Apply, except a Conflict is no longer refused:
// the existing target is renamed out of the way first (see backupPath),
// then the symlink is created as if the action had been Create.
func ApplyBackup(a Action) error {
	if a.Kind != Conflict {
		return Apply(a)
	}

	backup, err := backupPath(a.Target)
	if err != nil {
		return fmt.Errorf("%s: finding backup path for %s: %w", a.Name, a.Target, err)
	}
	if err := os.Rename(a.Target, backup); err != nil {
		return fmt.Errorf("%s: backing up %s: %w", a.Name, a.Target, err)
	}

	return Apply(Action{Name: a.Name, Source: a.Source, Target: a.Target, Kind: Create})
}

// backupPath finds a path near target to move the existing file to,
// trying "<target>.bak" first and then "<target>.bak.1", "<target>.bak.2",
// and so on until it finds one that isn't already taken.
func backupPath(target string) (string, error) {
	candidate := target + ".bak"
	for i := 1; ; i++ {
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s.bak.%d", target, i)
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

// UnlinkKind describes what PlanUnlink decided needs to happen to a
// single file.
type UnlinkKind int

const (
	// Managed means the target is a symlink pointing at the source file,
	// so it's ours to remove.
	Managed UnlinkKind = iota
	// Absent means there's nothing at the target path; nothing to do.
	Absent
	// Foreign means something exists at the target but it isn't a
	// symlink to the source, so PlanUnlink won't touch it: it might be a
	// real file, or a symlink some other tool created.
	Foreign
)

func (k UnlinkKind) String() string {
	switch k {
	case Managed:
		return "remove"
	case Absent:
		return "absent"
	case Foreign:
		return "foreign"
	default:
		return "unknown"
	}
}

// UnlinkAction is one file's worth of unlink plan.
type UnlinkAction struct {
	Name   string
	Source string
	Target string
	Kind   UnlinkKind
}

// PlanUnlink looks at every top-level file in sourceDir and reports
// whether its corresponding target in targetDir is a symlink dotlink
// would have created, so it's safe to remove. Like Plan, it only reads
// the filesystem.
func PlanUnlink(sourceDir, targetDir string) ([]UnlinkAction, error) {
	names, err := sourceFiles(sourceDir)
	if err != nil {
		return nil, err
	}

	var actions []UnlinkAction
	for _, name := range names {
		source, err := filepath.Abs(filepath.Join(sourceDir, name))
		if err != nil {
			return nil, err
		}
		target := filepath.Join(targetDir, "."+name)

		kind, err := classifyUnlink(source, target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}

		actions = append(actions, UnlinkAction{Name: name, Source: source, Target: target, Kind: kind})
	}
	return actions, nil
}

func classifyUnlink(source, target string) (UnlinkKind, error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return Absent, nil
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
			return Managed, nil
		}
	}
	return Foreign, nil
}

// ApplyUnlink removes a's target if and only if it's Managed. Absent and
// Foreign are no-ops: there's nothing at the target, or there's something
// there that dotlink didn't create and won't delete.
func ApplyUnlink(a UnlinkAction) error {
	switch a.Kind {
	case Absent, Foreign:
		return nil
	case Managed:
		if err := os.Remove(a.Target); err != nil {
			return fmt.Errorf("%s: removing %s: %w", a.Name, a.Target, err)
		}
		return nil
	default:
		return fmt.Errorf("%s: unknown unlink kind %v", a.Name, a.Kind)
	}
}
