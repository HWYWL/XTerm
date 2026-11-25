// Package record writes a session's transcript: what the server sends, the
// commands submitted, and what the app did with the session, each line
// with the time it happened.
package record

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The markers a line of the transcript starts with, after its time.
const (
	Output  = "OUTPUT"
	Command = "COMMAND"
	System  = "SYSTEM"
)

// Recorder is a session's transcript, being written.
type Recorder struct {
	mu      sync.Mutex
	f       *os.File
	w       *bufio.Writer
	path    string
	start   time.Time
	written int64
	closed  bool

	// The line being read: a terminal sends a typed character at a time,
	// so a line is written only once its newline arrives.
	pending   []rune
	pendingCR bool // a carriage return waits for the next character
}

// Dir is where the transcripts are kept: <config>/recordings.
func Dir(configDir string) string { return filepath.Join(configDir, "recordings") }

// File is where a session's transcript starts: <dir>/<session>/<date>/<HH-mm-ss>.log,
// with a suffix when that name is taken.
func File(dir, session string, now time.Time) string {
	day := filepath.Join(dir, safe(session), now.Format("2006-01-02"))
	name := now.Format("15-04-05")
	path := filepath.Join(day, name+".log")
	for i := 2; ; i++ {
		if _, err := os.Stat(path); err != nil {
			return path
		}
		path = filepath.Join(day, fmt.Sprintf("%s-%d.log", name, i))
	}
}

// Start opens path, writing header as the transcript's first lines.
func Start(path string, header []string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	r := &Recorder{f: f, w: bufio.NewWriter(f), path: path, start: time.Now()}
	r.line("========== XTerm Session ==========")
	for _, h := range header {
		r.line(h)
	}
	r.line("===================================")
	r.flush()
	return r, nil
}

// Path is the file written.
func (r *Recorder) Path() string { return r.path }

// Written is how many bytes were written.
func (r *Recorder) Written() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.written
}

// Started is when the recording began.
func (r *Recorder) Started() time.Time { return r.start }

// Output writes what the server sent: each line, with its time, after the
// terminal's control sequences are dropped. A typed character arrives on
// its own, so the line waits in pending until its newline comes; a
// carriage return that does not end a line starts it again, and a
// backspace takes the character back.
func (r *Recorder) Output(p []byte) {
	if len(p) == 0 {
		return
	}
	text := string(StripANSI(p))
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	for _, c := range text {
		if r.pendingCR {
			if c == '\n' {
				r.emitPending()
				continue
			}
			r.pending = r.pending[:0] // the line was written over
			r.pendingCR = false
		}
		switch c {
		case '\n':
			r.emitPending()
		case '\r':
			r.pendingCR = true
		case '\b':
			if n := len(r.pending); n > 0 {
				r.pending = r.pending[:n-1]
			}
		case '\t':
			r.pending = append(r.pending, c)
		default:
			if c >= 0x20 {
				r.pending = append(r.pending, c)
			}
		}
	}
}

// emitPending writes the line being read; the caller holds the lock.
func (r *Recorder) emitPending() {
	line := strings.TrimRight(string(r.pending), " \t")
	r.pending = r.pending[:0]
	if strings.TrimSpace(line) == "" {
		return
	}
	r.entryLocked(Output, line)
}

// Command writes a line the user submitted, after the line the terminal
// echoed for it.
func (r *Recorder) Command(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if !r.pendingCR {
		r.emitPending()
	}
	r.entryLocked(Command, line)
}

// Note writes something the app did with the session: it started, it was
// disconnected, the recording stopped.
func (r *Recorder) Note(text string) {
	if text == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.emitPending()
	r.entryLocked(System, text)
}

// Stop writes how the recording ended and closes the file.
func (r *Recorder) Stop(reason string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	// Whatever the screen still shows without a newline ends the
	// transcript as its last output line.
	r.emitPending()
	r.mu.Unlock()

	// The reason is a note like the others, written while the file is
	// still open.
	if reason != "" {
		r.Note(reason)
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()
	r.line(fmt.Sprintf("========== Session ended %s (%s) ==========",
		time.Now().Format("2006-01-02 15:04:05"), time.Since(r.start).Round(time.Second)))
	r.flush()
	_ = r.f.Close()
}

// entryLocked writes one line, prefixed with its time and what it is; the
// caller holds the lock.
func (r *Recorder) entryLocked(kind, text string) {
	r.line(fmt.Sprintf("[%s] [%s] %s", time.Now().Format("2006-01-02 15:04:05.000"), kind, text))
	r.flush()
}

// line writes text and a newline, without the lock.
func (r *Recorder) line(text string) {
	n, _ := r.w.WriteString(text + "\n")
	r.written += int64(n)
}

func (r *Recorder) flush() { _ = r.w.Flush() }

// safe makes a session's name a folder's: no separators, no characters
// Windows refuses.
func safe(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "session"
	}
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.Trim(b.String(), " .")
	if s == "" {
		return "session"
	}
	return s
}
