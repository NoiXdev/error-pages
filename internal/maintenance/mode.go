// Package maintenance decides whether the service is in maintenance mode.
// The mode is on when the environment flag is set or the flag file exists.
package maintenance

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ttl         = time.Second
	maxFlagRead = 1024 // bytes of the flag file that are ever read
)

// State is the current maintenance state. Until is zero when unknown or
// already in the past; otherwise it is in UTC.
type State struct {
	On    bool
	Until time.Time
}

// fileInfo is what the last look at the flag file found.
type fileInfo struct {
	exists bool
	until  time.Time // zero unless the content held a valid RFC 3339 time
}

// Mode reports the maintenance state. It is safe for concurrent use.
type Mode struct {
	envOn    bool
	envUntil time.Time
	file     string
	now      func() time.Time

	mu      sync.Mutex
	checked bool
	lastAt  time.Time
	cached  fileInfo
}

// New returns a Mode. envUntil is zero when unset.
func New(envOn bool, envUntil time.Time, file string, now func() time.Time) *Mode {
	if now == nil {
		now = time.Now
	}
	return &Mode{envOn: envOn, envUntil: envUntil, file: file, now: now}
}

// State returns the current state, looking at the flag file at most once
// per second.
func (m *Mode) State() State {
	now := m.now()

	m.mu.Lock()
	if !m.checked || now.Sub(m.lastAt) >= ttl || now.Before(m.lastAt) {
		m.cached = readFlag(m.file)
		m.lastAt = now
		m.checked = true
	}
	fi := m.cached
	m.mu.Unlock()

	if !m.envOn && !fi.exists {
		return State{}
	}
	until := fi.until
	if until.IsZero() {
		until = m.envUntil
	}
	if !until.After(now) {
		until = time.Time{}
	} else {
		until = until.UTC()
	}
	return State{On: true, Until: until}
}

// readFlag inspects the flag file. Any error other than "does not exist"
// counts as present, so an unreadable flag still switches the mode on.
func readFlag(path string) fileInfo {
	if path == "" {
		return fileInfo{}
	}
	st, err := os.Stat(path)
	if err != nil {
		return fileInfo{exists: !errors.Is(err, os.ErrNotExist)}
	}
	if !st.Mode().IsRegular() || st.Size() > maxFlagRead {
		return fileInfo{exists: true}
	}
	f, err := os.Open(path)
	if err != nil {
		return fileInfo{exists: !errors.Is(err, os.ErrNotExist)}
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxFlagRead+1))
	if err != nil || len(b) > maxFlagRead {
		return fileInfo{exists: true}
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil {
		return fileInfo{exists: true}
	}
	return fileInfo{exists: true, until: t}
}

// Switch turns the flag file on or off. Turning on writes until (RFC 3339,
// or empty) atomically via a temp file in the same directory; the parent
// directory must already exist. Turning off removes the file; a missing
// file is fine.
func Switch(file string, on bool, until string) error {
	if !on {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	var content string
	if until != "" {
		if _, err := time.Parse(time.RFC3339, until); err != nil {
			return fmt.Errorf("until must be RFC 3339: %w", err)
		}
		content = until + "\n"
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".maintenance-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, file); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// Describe renders the state for the CLI: "on", "on until <RFC 3339>" or "off".
func Describe(m *Mode) string {
	s := m.State()
	switch {
	case !s.On:
		return "off"
	case s.Until.IsZero():
		return "on"
	default:
		return "on until " + s.Until.Format(time.RFC3339)
	}
}
