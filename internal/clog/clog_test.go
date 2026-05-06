package clog_test

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/philspins/opendocket/internal/clog"
)

func restoreLog(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		clog.SetLevel(clog.LevelInfo)
	})
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0)
	return &buf
}

func TestSetLevel_DefaultIsInfo(t *testing.T) {
	clog.SetLevel(clog.LevelInfo)
	if clog.IsDebug() {
		t.Error("IsDebug() should return false at LevelInfo")
	}
}

func TestSetLevel_Debug(t *testing.T) {
	restoreLog(t)
	clog.SetLevel(clog.LevelDebug)
	if !clog.IsDebug() {
		t.Error("IsDebug() should return true at LevelDebug")
	}
}

func TestInfof_AlwaysEmitsAtInfoLevel(t *testing.T) {
	restoreLog(t)
	buf := captureLog(t)

	clog.SetLevel(clog.LevelInfo)
	clog.Infof("info %s %d", "msg", 42)

	if !strings.Contains(buf.String(), "info msg 42") {
		t.Errorf("Infof did not emit at LevelInfo, got: %q", buf.String())
	}
}

func TestInfof_AlwaysEmitsAtDebugLevel(t *testing.T) {
	restoreLog(t)
	buf := captureLog(t)

	clog.SetLevel(clog.LevelDebug)
	clog.Infof("always visible")

	if !strings.Contains(buf.String(), "always visible") {
		t.Errorf("Infof did not emit at LevelDebug, got: %q", buf.String())
	}
}

func TestDebugf_SuppressedAtInfoLevel(t *testing.T) {
	restoreLog(t)
	buf := captureLog(t)

	clog.SetLevel(clog.LevelInfo)
	clog.Debugf("should not appear")

	if strings.Contains(buf.String(), "should not appear") {
		t.Error("Debugf should not emit at LevelInfo")
	}
}

func TestDebugf_EmittedAtDebugLevel(t *testing.T) {
	restoreLog(t)
	buf := captureLog(t)

	clog.SetLevel(clog.LevelDebug)
	clog.Debugf("debug %s", "visible")

	if !strings.Contains(buf.String(), "debug visible") {
		t.Errorf("Debugf should emit at LevelDebug, got: %q", buf.String())
	}
}

func TestIsDebug_ReflectsSetLevel(t *testing.T) {
	restoreLog(t)

	for _, tc := range []struct {
		level int
		want  bool
	}{
		{clog.LevelInfo, false},
		{clog.LevelDebug, true},
	} {
		clog.SetLevel(tc.level)
		if got := clog.IsDebug(); got != tc.want {
			t.Errorf("SetLevel(%d): IsDebug() = %v, want %v", tc.level, got, tc.want)
		}
	}
}
