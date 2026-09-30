// Package windowstate persists the main window frame across launches so the
// application reopens at the size, position, and maximised state the user left
// it in, as expected by the macOS, Windows, and GNOME interface guidelines.
package windowstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const stateFilename = "window-state.json"

// Frame is a window position and size in logical pixels.
type Frame struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// State is the persisted window state for the next launch.
type State struct {
	Frame     Frame `json:"frame"`
	Maximised bool  `json:"maximised"`
}

// Path returns the state file location inside the application data directory.
func Path(dataDir string) string {
	return filepath.Join(dataDir, stateFilename)
}

// Load reads the persisted state. A missing file is not an error and reports
// ok=false so callers fall back to default window options. Negative
// coordinates are legitimate on multi-monitor desktops, but a non-positive
// size means the file does not describe a usable frame and is ignored.
func Load(path string) (state State, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("read window state: %w", err)
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return State{}, false, fmt.Errorf("parse window state: %w", err)
	}
	if state.Frame.Width <= 0 || state.Frame.Height <= 0 {
		return State{}, false, nil
	}
	return state, true, nil
}

// Save writes state atomically: a temporary file is written with private
// permissions and renamed over the previous state, so a crash mid-write never
// truncates the previous launch's state.
func Save(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode window state: %w", err)
	}
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create window state directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+stateFilename+"-*")
	if err != nil {
		return fmt.Errorf("create window state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup; after a successful rename there is nothing to remove.
	defer os.Remove(tmpName)
	if _, err = tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write window state: %w", err)
	}
	if err = tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("set window state permissions: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close window state temporary file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace window state: %w", err)
	}
	return nil
}
