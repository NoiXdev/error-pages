package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time       { return c.t }
func (c *clock) add(d time.Duration)  { c.t = c.t.Add(d) }
func at(h int) time.Time              { return time.Date(2026, 11, 10, h, 0, 0, 0, time.UTC) }
func flag(t *testing.T) string        { return filepath.Join(t.TempDir(), "maintenance") }
func write(t *testing.T, p, s string) { t.Helper(); mustNil(t, os.WriteFile(p, []byte(s), 0o644)) }
func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOffByDefault(t *testing.T) {
	c := &clock{at(17)}
	m := New(false, time.Time{}, flag(t), c.now)
	if s := m.State(); s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestEnvOn(t *testing.T) {
	c := &clock{at(17)}
	m := New(true, time.Time{}, flag(t), c.now)
	if s := m.State(); !s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestFileOnOffWithinTTL(t *testing.T) {
	f := flag(t)
	c := &clock{at(17)}
	m := New(false, time.Time{}, f, c.now)
	if m.State().On {
		t.Fatal("want off")
	}
	write(t, f, "")
	c.add(1100 * time.Millisecond)
	if !m.State().On {
		t.Fatal("want on after create")
	}
	mustNil(t, os.Remove(f))
	c.add(500 * time.Millisecond)
	if !m.State().On {
		t.Fatal("want cached on")
	}
	c.add(600 * time.Millisecond)
	if m.State().On {
		t.Fatal("want off after ttl")
	}
}

func TestFileUntil(t *testing.T) {
	f := flag(t)
	write(t, f, "2026-11-10T18:00:00Z\n")
	c := &clock{at(17)}
	s := New(false, time.Time{}, f, c.now).State()
	if !s.On || !s.Until.Equal(at(18)) || s.Until.Location() != time.UTC {
		t.Fatalf("got %+v", s)
	}
	c = &clock{at(19)}
	s = New(false, time.Time{}, f, c.now).State()
	if !s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestFileUntilNonUTCOffsetReturnedInUTC(t *testing.T) {
	f := flag(t)
	write(t, f, "2026-11-10T19:00:00+01:00\n")
	c := &clock{at(17)}
	s := New(false, time.Time{}, f, c.now).State()
	if !s.Until.Equal(at(18)) || s.Until.Location() != time.UTC {
		t.Fatalf("got %+v", s)
	}
}

func TestFileGarbageOrEmpty(t *testing.T) {
	for _, content := range []string{"soon", ""} {
		f := flag(t)
		write(t, f, content)
		c := &clock{at(17)}
		if s := New(false, time.Time{}, f, c.now).State(); !s.On || !s.Until.IsZero() {
			t.Fatalf("%q: got %+v", content, s)
		}
	}
}

func TestEnvUntilFallback(t *testing.T) {
	f := flag(t)
	write(t, f, "")
	c := &clock{at(17)}
	s := New(false, at(18), f, c.now).State()
	if !s.On || !s.Until.Equal(at(18)) {
		t.Fatalf("got %+v", s)
	}
	c = &clock{at(19)}
	if s := New(false, at(18), f, c.now).State(); !s.Until.IsZero() {
		t.Fatalf("past env until kept: %+v", s)
	}
}

func TestFileUntilBeatsEnvUntil(t *testing.T) {
	f := flag(t)
	write(t, f, "2026-11-10T18:00:00Z")
	c := &clock{at(17)}
	s := New(true, at(20), f, c.now).State()
	if !s.Until.Equal(at(18)) {
		t.Fatalf("got %+v", s)
	}
}

func TestLargeFileIsOnWithoutEnd(t *testing.T) {
	f := flag(t)
	write(t, f, "2026-11-10T18:00:00Z\n"+strings.Repeat("x", 2048))
	c := &clock{at(17)}
	if s := New(false, time.Time{}, f, c.now).State(); !s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestDirectoryIsOnWithoutEnd(t *testing.T) {
	c := &clock{at(17)}
	if s := New(false, time.Time{}, t.TempDir(), c.now).State(); !s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestUnreadableFileIsOn(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	f := flag(t)
	write(t, f, "2026-11-10T18:00:00Z")
	mustNil(t, os.Chmod(f, 0o000))
	c := &clock{at(17)}
	if s := New(false, time.Time{}, f, c.now).State(); !s.On || !s.Until.IsZero() {
		t.Fatalf("got %+v", s)
	}
}

func TestConcurrentState(t *testing.T) {
	f := flag(t)
	write(t, f, "")
	m := New(false, time.Time{}, f, time.Now)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				m.State()
			}
		}()
	}
	wg.Wait()
}

func TestSwitch(t *testing.T) {
	f := flag(t)
	mustNil(t, Switch(f, true, ""))
	b, err := os.ReadFile(f)
	mustNil(t, err)
	if len(b) != 0 {
		t.Fatalf("want empty, got %q", b)
	}
	if fi, _ := os.Stat(f); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if Switch(f, true, "bad") == nil {
		t.Fatal("want error for bad until")
	}
	mustNil(t, Switch(f, true, "2026-11-10T18:00:00Z"))
	b, _ = os.ReadFile(f)
	if string(b) != "2026-11-10T18:00:00Z\n" {
		t.Fatalf("got %q", b)
	}
	mustNil(t, Switch(f, false, ""))
	mustNil(t, Switch(f, false, ""))
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file should be gone")
	}
}

func TestSwitchOnMissingDirErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "nope", "maintenance")
	if Switch(f, true, "") == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Dir(f)); err == nil {
		t.Fatal("directory must not be created")
	}
}

func TestSwitchLeavesNoTempFiles(t *testing.T) {
	d := t.TempDir()
	mustNil(t, Switch(filepath.Join(d, "maintenance"), true, ""))
	es, _ := os.ReadDir(d)
	if len(es) != 1 {
		t.Fatalf("got %d entries", len(es))
	}
}

func TestDescribe(t *testing.T) {
	f := flag(t)
	c := &clock{at(17)}
	m := New(false, time.Time{}, f, c.now)
	if got := Describe(m); got != "off" {
		t.Fatal(got)
	}
	write(t, f, "")
	c.add(2 * time.Second)
	if got := Describe(m); got != "on" {
		t.Fatal(got)
	}
	write(t, f, "2026-11-10T18:00:00Z")
	c.add(2 * time.Second)
	if got := Describe(m); got != "on until 2026-11-10T18:00:00Z" {
		t.Fatal(got)
	}
}
