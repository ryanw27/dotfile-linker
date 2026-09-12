package dotlink

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestPlanAndApply(t *testing.T) {
	tests := []struct {
		name string
		// setup writes name.txt into sourceDir and prepares targetDir's
		// ".name.txt" however the case needs. It returns the mapping to
		// pass to Plan, or nil for the default.
		setup    func(t *testing.T, sourceDir, targetDir string) map[string]string
		wantKind ActionKind
		// wantLinked, if true, is checked after Apply: the target must be
		// a symlink pointing at the source file.
		wantLinked bool
	}{
		{
			name: "create when target absent",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
				return nil
			},
			wantKind:   Create,
			wantLinked: true,
		},
		{
			name: "noop when already linked",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				source := filepath.Join(sourceDir, "name.txt")
				writeFile(t, source, "hello\n")
				if err := os.Symlink(source, filepath.Join(targetDir, ".name.txt")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return nil
			},
			wantKind:   NoOp,
			wantLinked: true,
		},
		{
			name: "relink when symlinked elsewhere",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
				elsewhere := filepath.Join(targetDir, "elsewhere.txt")
				writeFile(t, elsewhere, "other\n")
				if err := os.Symlink(elsewhere, filepath.Join(targetDir, ".name.txt")); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return nil
			},
			wantKind:   Relink,
			wantLinked: true,
		},
		{
			name: "relink when regular file has identical content",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
				writeFile(t, filepath.Join(targetDir, ".name.txt"), "hello\n")
				return nil
			},
			wantKind:   Relink,
			wantLinked: true,
		},
		{
			name: "conflict when regular file differs",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
				writeFile(t, filepath.Join(targetDir, ".name.txt"), "local edits\n")
				return nil
			},
			wantKind:   Conflict,
			wantLinked: false,
		},
		{
			name: "mapping overrides default target name",
			setup: func(t *testing.T, sourceDir, targetDir string) map[string]string {
				writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
				return map[string]string{"name.txt": "config/name/config.txt"}
			},
			wantKind:   Create,
			wantLinked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceDir := t.TempDir()
			targetDir := t.TempDir()
			mapping := tt.setup(t, sourceDir, targetDir)

			actions, err := Plan(sourceDir, targetDir, mapping)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if len(actions) != 1 {
				t.Fatalf("Plan returned %d actions, want 1", len(actions))
			}
			a := actions[0]
			if a.Kind != tt.wantKind {
				t.Fatalf("Plan kind = %v, want %v", a.Kind, tt.wantKind)
			}

			err = Apply(a)
			if tt.wantKind == Conflict {
				if err == nil {
					t.Fatal("Apply on a conflict succeeded, want error")
				}
				if _, statErr := os.Lstat(a.Target); statErr != nil {
					t.Fatalf("conflicting target should be untouched: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}

			if tt.wantLinked {
				dest, err := os.Readlink(a.Target)
				if err != nil {
					t.Fatalf("target is not a symlink after Apply: %v", err)
				}
				if dest != a.Source {
					t.Fatalf("symlink points at %s, want %s", dest, a.Source)
				}
			}

			// Applying an already-applied action should now report NoOp
			// and Apply should be a harmless no-op.
			again, err := Plan(sourceDir, targetDir, mapping)
			if err != nil {
				t.Fatalf("second Plan: %v", err)
			}
			if again[0].Kind != NoOp {
				t.Fatalf("second Plan kind = %v, want NoOp", again[0].Kind)
			}
			if err := Apply(again[0]); err != nil {
				t.Fatalf("second Apply: %v", err)
			}
		})
	}
}

func TestApplyBackupPreservesConflictingContent(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	writeFile(t, filepath.Join(sourceDir, "name.txt"), "hello\n")
	writeFile(t, filepath.Join(targetDir, ".name.txt"), "local edits\n")

	actions, err := Plan(sourceDir, targetDir, nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	a := actions[0]
	if a.Kind != Conflict {
		t.Fatalf("Plan kind = %v, want Conflict", a.Kind)
	}

	if err := ApplyBackup(a); err != nil {
		t.Fatalf("ApplyBackup: %v", err)
	}

	dest, err := os.Readlink(a.Target)
	if err != nil {
		t.Fatalf("target is not a symlink after ApplyBackup: %v", err)
	}
	if dest != a.Source {
		t.Fatalf("symlink points at %s, want %s", dest, a.Source)
	}

	backup := a.Target + ".bak"
	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("reading backup file: %v", err)
	}
	if string(got) != "local edits\n" {
		t.Fatalf("backup content = %q, want %q", got, "local edits\n")
	}
}
