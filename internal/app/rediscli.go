package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"finallyshell/internal/redis"
	"finallyshell/locales"
)

// The Redis tab is a terminal whose connection is a Redis server: the
// commands typed there go to the server, and its replies come back as
// redis-cli writes them. redisCLI is that connection for one pane: it reads
// the keys the terminal sends, keeps the line, and runs the commands on a
// goroutine of its own, so that the window never waits for a reply.

// redisTimeout bounds one command, as a server that hangs does not hold
// the tab.
const redisTimeout = 30 * time.Second

// redisCLI is a terminal's connection to a Redis server: what the terminal
// writes is typed, what it reads is shown.
type redisCLI struct {
	ctx    context.Context
	cancel context.CancelFunc
	client *redis.Client
	db     int
	// label is the address the prompt shows.
	label string
	// out is what the terminal shows; writes go through a goroutine, as
	// the window must not wait for the pipe.
	outR *io.PipeReader
	outW *io.PipeWriter
	out  chan []byte
	// cmds are the lines to run, in order.
	cmds chan string
	// line is what is typed so far, as the bytes of its text, and pos
	// where the cursor is in it — after a whole character, never inside
	// one, so that 中文 is typed and sent as it is.
	line []byte
	pos  int
	// pending is the bytes of a character not complete yet, as a
	// character of several bytes may come key by key.
	pending []byte
	// esc says a key sequence is being read: 1 after ESC, 2 after ESC [,
	// and 3 in the parameters of one, as ESC [ 3 ~.
	esc    int
	params string
	// hist is what was run, and histAt where the arrows are in it.
	hist   []string
	histAt int
	// quitFlag says the command line left — `quit`, `exit`, or Ctrl+D on
	// an empty line — rather than the server dropping it.
	quitFlag atomic.Bool
	// stop ends the goroutines that write and run; it is closed once, and
	// nothing sends after it, as a channel closed with a sender panics.
	stop chan struct{}
	once sync.Once
}

// newRedisCLI is the connection of a Redis tab: client reaches the server,
// label is where the prompt says it is.
func newRedisCLI(ctx context.Context, client *redis.Client, label string, db int) *redisCLI {
	ctx, cancel := context.WithCancel(ctx)
	c := &redisCLI{
		ctx: ctx, cancel: cancel, client: client, label: label, db: db,
		out: make(chan []byte, 1024), cmds: make(chan string, 64),
		stop: make(chan struct{}),
	}
	c.outR, c.outW = io.Pipe()
	go c.writer()
	go c.run()
	c.print("\x1b[2m" + locales.RedisConnected(label) + "\x1b[0m\r\n")
	c.printPrompt()
	return c
}

// writer is what the terminal reads, from the channel the rest fills. It
// writes what was said before the connection closed, so that the last
// answer, or the goodbye, is not lost, and then its end.
func (c *redisCLI) writer() {
	for {
		select {
		case b := <-c.out:
			if _, err := c.outW.Write(b); err != nil {
				return
			}
		case <-c.stop:
			for {
				select {
				case b := <-c.out:
					if _, err := c.outW.Write(b); err != nil {
						return
					}
				default:
					c.outW.Close()
					return
				}
			}
		}
	}
}

// print shows s in the terminal; a full channel drops the text rather than
// holding the window, and nothing is shown once the connection is closed.
func (c *redisCLI) print(s string) {
	select {
	case c.out <- []byte(s):
	case <-c.stop:
	default:
	}
}

// Read is what the terminal shows.
func (c *redisCLI) Read(p []byte) (int, error) { return c.outR.Read(p) }

// Close ends the connection and what runs commands.
func (c *redisCLI) Close() error {
	c.once.Do(func() {
		c.cancel()
		close(c.stop) // the writer writes what is left, then closes the pipe
		c.client.Close()
	})
	return nil
}

// Write is what is typed: the keys are kept on the line and shown as they
// are, as a shell's are.
func (c *redisCLI) Write(p []byte) (int, error) {
	for _, b := range p {
		c.key(b)
	}
	return len(p), nil
}

// key takes one byte typed.
func (c *redisCLI) key(b byte) {
	if c.esc == 2 { // a key after ESC [
		switch {
		case b >= '0' && b <= '9', b == ';':
			c.esc, c.params = 3, c.params+string(b)
		case b == '~':
			c.escapeKey(c.params + "~")
		default:
			c.escapeKey(string(b))
		}
		return
	}
	if c.esc == 3 { // the parameters of a key, then its final byte
		switch {
		case b >= '0' && b <= '9', b == ';':
			c.params += string(b)
		case b == '~':
			c.escapeKey(c.params + "~")
		default:
			c.escapeKey(c.params + string(b))
		}
		return
	}
	if c.esc == 4 { // a key after ESC O, as the arrows of some terminals
		c.esc = 0
		c.escapeKey(string(b))
		return
	}
	if c.esc == 1 { // a key after ESC
		switch b {
		case '[':
			c.esc, c.params = 2, ""
		case 'O':
			c.esc = 4
		default:
			c.esc = 0
		}
		return
	}
	if c.pending != nil {
		// A character of several bytes: it is typed once it is whole.
		c.pending = append(c.pending, b)
		if utf8.FullRune(c.pending) {
			if r, _ := utf8.DecodeRune(c.pending); r != utf8.RuneError {
				c.insert(string(c.pending))
			}
			c.pending = nil
		} else if len(c.pending) > utf8.UTFMax {
			c.pending = nil
		}
		return
	}
	if b >= utf8.RuneSelf {
		c.pending = []byte{b}
		return
	}
	switch b {
	case 0x1b:
		c.esc = 1
	case '\r', '\n':
		c.enter()
	case 0x7f, 0x08:
		c.backspace()
	case 0x01: // Ctrl+A: the start of the line
		c.pos = 0
		c.redraw()
	case 0x05: // Ctrl+E: its end
		c.pos = len(c.line)
		c.redraw()
	case 0x0b: // Ctrl+K: what follows the cursor is cut
		c.line = c.line[:c.pos]
		c.redraw()
	case 0x15: // Ctrl+U: the line is cut
		c.line, c.pos = nil, 0
		c.redraw()
	case 0x03: // Ctrl+C: what is typed is dropped
		c.print("^C\r\n")
		c.line, c.pos = nil, 0
		c.printPrompt()
	case 0x04: // Ctrl+D deletes at the cursor, and closes an empty line
		if len(c.line) == 0 {
			c.quit()
			return
		}
		c.delete()
	case 0x0c: // Ctrl+L: the screen is cleared
		c.print("\x1b[2J\x1b[H")
		c.printPrompt()
		c.redraw()
	default:
		if b < 0x20 {
			return
		}
		c.insert(string(b))
	}
}

// escapeKey takes a key written as Escape and parameters: the arrows, Home
// and End, and Delete.
func (c *redisCLI) escapeKey(seq string) {
	c.esc, c.params = 0, ""
	switch seq {
	case "A":
		c.history(-1)
	case "B":
		c.history(1)
	case "C": // right
		c.move(1)
	case "D": // left
		c.move(-1)
	case "H", "1~", "7~": // Home
		c.pos = 0
		c.redraw()
	case "F", "4~", "8~": // End
		c.pos = len(c.line)
		c.redraw()
	case "3~": // Delete, forward
		c.delete()
	default:
		// Ctrl+Left and the like are the key with a modifier: the key
		// still moves the cursor.
		if i := strings.IndexByte(seq, ';'); i >= 0 && i < len(seq)-2 {
			if last := seq[len(seq)-1]; last == 'A' || last == 'B' || last == 'C' ||
				last == 'D' || last == 'H' || last == 'F' {
				c.escapeKey(string(last))
			}
		}
	}
}

// insert types s where the cursor is.
func (c *redisCLI) insert(s string) {
	c.line = slices.Insert(c.line, c.pos, []byte(s)...)
	c.pos += len(s)
	c.redraw()
}

// backspace deletes the character before the cursor.
func (c *redisCLI) backspace() {
	if c.pos == 0 {
		return
	}
	_, size := utf8.DecodeLastRune(c.line[:c.pos])
	c.line = append(c.line[:c.pos-size], c.line[c.pos:]...)
	c.pos -= size
	c.redraw()
}

// delete deletes the character at the cursor.
func (c *redisCLI) delete() {
	if c.pos >= len(c.line) {
		return
	}
	_, size := utf8.DecodeRune(c.line[c.pos:])
	c.line = append(c.line[:c.pos], c.line[c.pos+size:]...)
	c.redraw()
}

// move puts the cursor on the character after (direction 1) or before (-1)
// it, so that an arrow never lands inside 中文.
func (c *redisCLI) move(direction int) {
	if direction < 0 {
		if c.pos == 0 {
			return
		}
		_, size := utf8.DecodeLastRune(c.line[:c.pos])
		c.pos -= size
	} else {
		if c.pos >= len(c.line) {
			return
		}
		_, size := utf8.DecodeRune(c.line[c.pos:])
		c.pos += size
	}
	c.redraw()
}

// prompt is what the line starts with: where the server is, and the
// database when it is not the first.
func (c *redisCLI) prompt() string {
	if c.db > 0 {
		return fmt.Sprintf("%s[%d]> ", c.label, c.db)
	}
	return c.label + "> "
}

// printPrompt starts a line.
func (c *redisCLI) printPrompt() { c.print(c.prompt()) }

// redraw writes the line again, as an edit, the history or the cursor
// moved it, and puts the cursor back where it is: the columns after it are
// counted in the cells the characters take, as 中文 takes two each.
func (c *redisCLI) redraw() {
	s := "\r\x1b[K" + c.prompt() + string(c.line)
	if n := cells(c.line[c.pos:]); n > 0 {
		s += fmt.Sprintf("\x1b[%dD", n)
	}
	c.print(s)
}

// cells is how many columns s takes in a terminal.
func cells(s []byte) int {
	n := 0
	for _, r := range string(s) {
		n += runeCells(r)
	}
	return n
}

// runeCells is how many columns r takes: two for the wide characters of
// Chinese, Japanese and Korean, and for emoji; none for a combining mark.
func runeCells(r rune) int {
	switch {
	case r < 0x20 || (r >= 0x7f && r < 0xa0), r == 0x200b:
		return 0
	case unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r):
		return 0
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xa960 && r <= 0xa97f,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe10 && r <= 0xfe19,
		r >= 0xfe30 && r <= 0xfe6f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}

// enter runs the line, which is empty enough to be nothing.
func (c *redisCLI) enter() {
	line := strings.TrimSpace(string(c.line))
	c.print("\r\n")
	c.line, c.pos = nil, 0
	if line == "" {
		c.printPrompt()
		return
	}
	if n := len(c.hist); n == 0 || c.hist[n-1] != line {
		c.hist = append(c.hist, line)
	}
	c.histAt = len(c.hist)
	select {
	case c.cmds <- line:
	default:
		c.print("\x1b[31m(error) busy\x1b[0m\r\n")
		c.printPrompt()
	}
}

// history puts the command at direction (-1 back, 1 on) on the line.
func (c *redisCLI) history(direction int) {
	at := c.histAt + direction
	if at < 0 || at > len(c.hist) {
		return
	}
	c.histAt = at
	if at == len(c.hist) {
		c.line, c.pos = nil, 0
	} else {
		c.line, c.pos = []byte(c.hist[at]), len(c.hist[at])
	}
	c.redraw()
}

// run runs the lines in order, one at a time.
func (c *redisCLI) run() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case line := <-c.cmds:
			c.execute(line)
			c.printPrompt()
		}
	}
}

// execute runs one line: the CLI's own commands, or one of the server's.
func (c *redisCLI) execute(line string) {
	args, err := splitRedisCommand(line)
	if err != nil {
		c.print("\x1b[31m(error) " + err.Error() + "\x1b[0m\r\n")
		return
	}
	if len(args) == 0 {
		return
	}
	selected := -1
	switch strings.ToLower(args[0]) {
	case "quit", "exit":
		c.print("\x1b[2m" + locales.RedisBye() + "\x1b[0m\r\n")
		c.quit()
		return
	case "clear":
		c.print("\x1b[2J\x1b[H")
		return
	case "help":
		c.print("\x1b[2m" + redisHelp() + "\x1b[0m\r\n")
		return
	case "select":
		if len(args) == 2 {
			if n, err := strconv.Atoi(args[1]); err == nil {
				selected = max(n, 0)
			}
		}
	}
	ctx, cancel := context.WithTimeout(c.ctx, redisTimeout)
	defer cancel()
	reply, err := c.client.Do(ctx, args...)
	if err != nil {
		if c.ctx.Err() != nil {
			return
		}
		c.print("\x1b[31m(error) " + err.Error() + "\x1b[0m\r\n")
		return
	}
	c.print(formatRedisReply(reply))
	if selected >= 0 && reply.Kind != redis.Err {
		c.db = selected
	}
}

// quit closes the terminal's connection: the tab shows the end.
func (c *redisCLI) quit() {
	c.quitFlag.Store(true)
	c.Close()
}

// redisHelp is what the CLI says about itself.
func redisHelp() string {
	return strings.Join([]string{
		locales.RedisHelpIntro(),
		"  PING, INFO, KEYS *, GET key, SET key value, DEL key, TTL key",
		"  SCAN 0 MATCH * COUNT 100, TYPE key, DBSIZE, SELECT n",
		"  quit, clear, help",
	}, "\r\n")
}

// splitRedisCommand splits a line as redis-cli does: on spaces, with
// quotes around an argument that holds them, and a backslash to keep the
// character after it.
func splitRedisCommand(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	var quote byte
	started := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == '\\' && i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
				continue
			}
			if c == quote {
				quote = 0
				continue
			}
			cur.WriteByte(c)
		case c == '"' || c == '\'':
			quote, started = c, true
		case c == ' ' || c == '\t':
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			started = true
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}

// formatRedisReply writes a reply as redis-cli does: an array numbered and
// indented, a string quoted, and an error in red.
func formatRedisReply(r redis.Reply) string {
	var b strings.Builder
	writeRedisReply(&b, r, "", "")
	return b.String()
}

// writeRedisReply writes r, its first line after first and the others after
// rest: a nested array's items go under its number, as redis-cli shows
// them.
func writeRedisReply(b *strings.Builder, r redis.Reply, first, rest string) {
	if r.Kind != redis.Array {
		b.WriteString(first + redisScalar(r) + "\r\n")
		return
	}
	if len(r.Items) == 0 {
		b.WriteString(first + "(empty array)\r\n")
		return
	}
	width := len(strconv.Itoa(len(r.Items)))
	for i, item := range r.Items {
		num := fmt.Sprintf("%*d) ", width, i+1)
		p := rest
		if i == 0 {
			p = first
		}
		writeRedisReply(b, item, p+num, p+strings.Repeat(" ", len(num)))
	}
}

// redisScalar is one reply as a line, without its number.
func redisScalar(r redis.Reply) string {
	switch r.Kind {
	case redis.Status:
		return r.Str
	case redis.Err:
		return "\x1b[31m(error) " + r.Str + "\x1b[0m"
	case redis.Int:
		return "(integer) " + strconv.FormatInt(r.Int, 10)
	case redis.Bulk:
		return quoteRedisString(r.Str)
	case redis.Nil:
		return "(nil)"
	}
	return ""
}

// quoteRedisString quotes s as redis-cli does, escapes and all, but keeps
// the text of a character of several bytes: 中文 is shown as 中文, where
// redis-cli writes the bytes of it.
func quoteRedisString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 { // a byte that is no text
			fmt.Fprintf(&b, `\x%02x`, s[i])
			i++
			continue
		}
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteString(s[i : i+size])
			}
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}
