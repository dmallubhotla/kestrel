package logging

import (
	"log/slog"
	"os"

	"github.com/example/kestrel/internal/profile"
)

var logFile *os.File

// Init sets up file-based logging to $XDG_CACHE_HOME/kest/logs/kest.log.
// Returns a cleanup function to close the log file.
func Init() (func(), error) {
	dir := profile.LogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}, err
	}

	f, err := os.OpenFile(dir+"/kest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func() {}, err
	}
	logFile = f

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))

	return func() { f.Close() }, nil
}
