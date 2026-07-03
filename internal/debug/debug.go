package debug

import (
	"log"
	"os"
	"sync"
	"time"
)

var (
	mu     sync.Mutex
	logger *log.Logger
	file   *os.File
)

// Init opens path for writing and enables debug logging.
// The file is truncated on each run so old sessions don't accumulate.
func Init(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	file = f
	logger = log.New(f, "", 0)
	Log("=== session start %s ===", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}

// Enabled returns true when debug logging is active.
func Enabled() bool { return logger != nil }

// Log writes a formatted line with a millisecond-precision timestamp.
func Log(format string, args ...interface{}) {
	if logger == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	all := make([]interface{}, 0, 1+len(args))
	all = append(all, ts)
	all = append(all, args...)
	logger.Printf("[%s] "+format, all...)
}

// Close flushes and closes the log file.
func Close() {
	if file != nil {
		_ = file.Sync()
		_ = file.Close()
		file = nil
		logger = nil
	}
}
