package profile

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

const appName = "kest"

// StateDir returns the kest state directory ($XDG_STATE_HOME/kest/).
func StateDir() string {
	return filepath.Join(xdg.StateHome, appName)
}

// LogDir returns $XDG_CACHE_HOME/kest/logs/.
func LogDir() string {
	return filepath.Join(xdg.CacheHome, appName, "logs")
}

func activeProfilePath() string {
	return filepath.Join(StateDir(), "active-profile")
}

// Read returns the currently active environment name, or "" if none is set.
func Read() (string, error) {
	data, err := os.ReadFile(activeProfilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Write persists the given environment name as the active profile.
func Write(env string) error {
	dir := StateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(activeProfilePath(), []byte(env+"\n"), 0o644)
}
