package windowstate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), stateFilename)

	state, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if ok {
		t.Fatalf("Load() ok = true for missing file, state = %+v", state)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := Path(t.TempDir())
	want := State{
		// Negative coordinates are valid on multi-monitor desktops.
		Frame:     Frame{X: -1920, Y: 40, Width: 920, Height: 680},
		Maximised: true,
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() ok = false after Save()")
	}
	if got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeply", stateFilename)

	if err := Save(path, State{Frame: Frame{Width: 920, Height: 680}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file not created: %v", err)
	}
}

func TestSaveWritesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are advisory on Windows")
	}
	path := Path(t.TempDir())

	if err := Save(path, State{Frame: Frame{Width: 920, Height: 680}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file permissions = %o, want 600", perm)
	}
}

func TestLoadInvalidJSONFails(t *testing.T) {
	path := Path(t.TempDir())
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil for invalid JSON")
	}
}

func TestLoadRejectsUnusableSizes(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"zero width", 0, 680},
		{"negative width", -10, 680},
		{"zero height", 920, 0},
		{"negative height", 920, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := Path(t.TempDir())
			state := State{Frame: Frame{X: 10, Y: 20, Width: tt.width, Height: tt.height}}
			if err := Save(path, state); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, ok, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if ok {
				t.Fatalf("Load() ok = true for unusable size, state = %+v", got)
			}
		})
	}
}

func TestSaveLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	path := Path(dir)

	if err := Save(path, State{Frame: Frame{Width: 920, Height: 680}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != stateFilename {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("data directory contains %v, want only %q", names, stateFilename)
	}
}
