// Package clog provides levelled logging for the crawler.
// Call SetLevel once at startup; the zero value (LevelInfo) is safe to use.
package clog

import (
	"log"
	"sync/atomic"
)

const (
	LevelInfo  = 0
	LevelDebug = 1
)

var level atomic.Int32

// SetLevel configures the minimum log level. Thread-safe.
func SetLevel(l int) { level.Store(int32(l)) }

// IsDebug reports whether debug-level logging is enabled.
func IsDebug() bool { return level.Load() >= int32(LevelDebug) }

// Infof logs a message at INFO level. Always emitted regardless of log level.
func Infof(format string, args ...any) { log.Printf(format, args...) }

// Debugf logs a message at DEBUG level. Only emitted when --log-level=debug.
func Debugf(format string, args ...any) {
	if level.Load() >= int32(LevelDebug) {
		log.Printf(format, args...)
	}
}
