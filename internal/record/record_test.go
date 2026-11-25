package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecorder: the transcript has a header, one line per output line with
// its time and kind, the command submitted, and how it ended; the
// terminal's control sequences are gone.
func TestRecorder(t *testing.T) {
	dir := t.TempDir()
	path := File(dir, "web/1", time.Now())
	rec, err := Start(path, []string{"Session: web/1", "Host: root@10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	rec.Output([]byte("\x1b[32mhello\x1b[0m\r\nworld\n"))
	rec.Command("ls -la")
	rec.Stop("done")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"========== XTerm Session ==========",
		"Session: web/1",
		"[OUTPUT] hello",
		"[OUTPUT] world",
		"[COMMAND] ls -la",
		"[SYSTEM] done",
		"========== Session ended",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Error("a control sequence was kept")
	}
	if got := rec.Written(); got != int64(len(data)) {
		t.Errorf("written %d, the file has %d", got, len(data))
	}
}

// TestTypedLine: a terminal sends a typed character at a time, and the
// transcript still has one line for the line, not one per character; a
// backspace erases, as on the screen.
func TestTypedLine(t *testing.T) {
	rec, err := Start(File(t.TempDir(), "web", time.Now()), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range []string{"p", "w", "x", "\b", "d", "\r", "\n"} {
		rec.Output([]byte(ch))
	}
	rec.Stop("")
	data, err := os.ReadFile(rec.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "[OUTPUT] pwd") {
		t.Errorf("the line is not whole:\n%s", text)
	}
	if n := strings.Count(text, "[OUTPUT]"); n != 1 {
		t.Errorf("%d output lines:\n%s", n, text)
	}
}

// TestFileUnique: a transcript does not overwrite another started in the
// same second.
func TestFileUnique(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 25, 15, 4, 5, 0, time.Local)
	first := File(dir, "web", now)
	if err := os.MkdirAll(filepath.Dir(first), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	second := File(dir, "web", now)
	if second == first || !strings.HasSuffix(second, "-2.log") {
		t.Errorf("second %q, first %q", second, first)
	}
}

// TestStripANSI: the cursor moves, colors and titles go; the text stays.
func TestStripANSI(t *testing.T) {
	in := "\x1b[1;31mred\x1b[0m \x1b]0;title\x07text\x1b[2J\x1b[H"
	if got := string(StripANSI([]byte(in))); got != "red text" {
		t.Errorf("got %q", got)
	}
}

// TestSafe: a session's name makes a folder that any system accepts.
func TestSafe(t *testing.T) {
	if got := safe(`a/b:c*d?e"f<g>h|i`); got != "a_b_c_d_e_f_g_h_i" {
		t.Errorf("got %q", got)
	}
	if got := safe("  "); got != "session" {
		t.Errorf("got %q", got)
	}
}
