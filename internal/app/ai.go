package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"finallyshell/internal/ai"
	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The AI tab of a session's panel asks an OpenAI-compatible service about
// what the terminal shows, and about the host it is connected to: the
// model's answer streams in, its reasoning with it, and the tools it asks
// for run on the session it is the tab of. Each session has a conversation
// of its own, so that the tabs of different hosts do not mix.
//
// The request is answered on a goroutine of its own, which changes the page
// only through App.post; the pieces of an answer are posted at most every
// AIFlush, so that a fast stream does not redraw the window for every word.

const (
	// aiMaxTurns is how many turns of the conversation go with a request.
	aiMaxTurns = 8
	// aiMaxToolRounds is how many times a model may call tools in one turn
	// before the page stops running them.
	aiMaxToolRounds = 4
	// aiToolChars is how much of a tool's output the model gets.
	aiToolChars = 4000
	// aiFlush is how often the pieces of an answer are posted.
	aiFlush = 200 * time.Millisecond
)

// aiState is what the assistant keeps besides the settings: the chosen
// provider's key, as read from the store, and the options page's own state.
type aiState struct {
	// key is the chosen provider's API key, as read from the store, and
	// keyFor the provider it was read for.
	key       string
	keyFor    string
	keyLoaded bool
	// test says what the test of the provider's connection did, if it was
	// asked.
	test    string
	testing bool
	// newModel is the model the options page is about to add.
	newModel string
}

// aiChat is one session's conversation with the assistant: the turns, the
// request that answers the last of them, and what it waits for.
type aiChat struct {
	turns []aiTurn
	// busy is whether a request is out: the page takes no new question
	// while it is.
	busy bool
	// input is what the user is typing, sent when they send it.
	input string
	// err is what keeps the page from asking, as no provider or key.
	err string
	// cancel stops the request out there.
	cancel func()
	// seq numbers the requests: what one answers after another has taken
	// its place is dropped.
	seq int
	// list is the turns as a list, which follows the answer down.
	list ui.ListState
	// caret is where the caret is in input as the page was built, and
	// lastInput the text it had then: a newline typed with Ctrl+Enter goes
	// where the caret is, before the next frame shows it.
	caret     int
	lastInput string
	// caretSet puts the caret at newCaret in the next frame, after such a
	// newline.
	caretSet bool
	newCaret int
	// openReasoning is the turns whose reasoning shows.
	openReasoning map[int]bool
	// pending is a command the assistant asked to run, waiting for the
	// user.
	pending *aiPending
	// toolMsgs are the assistant's tool calls and their results, of the
	// turn being answered.
	toolMsgs []ai.Message
	// callQueue are the tool calls of the answer just read, still to run.
	callQueue []ai.ToolCall
}

// aiTurn is one question and its answer.
type aiTurn struct {
	prompt    string
	answer    string
	reasoning string
	// failed is why the answer did not come, if it did not.
	failed string
	// stopped is whether the user stopped it.
	stopped bool
	// tools is how many tools the answer called for.
	tools int
	// model is the provider and model that answered, as the page says.
	model string
	// blocks is the answer as the page shows it, parsed from renderedFrom
	// (see markdown.go).
	blocks       []mdBlock
	renderedFrom string
}

// aiPending is a command the assistant may run, waiting for the user.
type aiPending struct {
	call ai.ToolCall
	cmd  string
}

// ensureAIKey reads the chosen provider's key from the credential store,
// once for each provider.
func (a *App) ensureAIKey() {
	st := &a.ai
	p, ok := a.settings.AI.ChosenProvider()
	if !ok {
		return
	}
	if st.keyLoaded && st.keyFor == p.ID {
		return
	}
	st.keyFor, st.keyLoaded = p.ID, false
	key := config.AIKeyName(p.ID)
	a.secret(key, func(v string, _ bool) {
		if st.keyFor != p.ID {
			return // another provider was chosen meanwhile
		}
		st.key, st.keyLoaded = v, true
	})
}

// aiClient is the client of the chosen provider and model, and why there is
// none: "" when the page can talk to it.
func (a *App) aiClient() (*ai.Client, string) {
	st := &a.ai
	cfg := a.settings.AI
	p, ok := cfg.ChosenProvider()
	if !ok || strings.TrimSpace(p.BaseURL) == "" {
		return nil, locales.AiNoProvider()
	}
	m, ok := cfg.ChosenModel()
	if !ok || m.Provider != p.ID {
		return nil, locales.AiNoModel()
	}
	// A service on this machine needs no key, and no wait for one.
	if !st.keyLoaded || st.keyFor != p.ID {
		if !config.LocalEndpoint(p.BaseURL) {
			a.ensureAIKey()
			if !st.keyLoaded {
				return nil, locales.AiKeyLoading()
			}
		}
	}
	if st.key == "" && !config.LocalEndpoint(p.BaseURL) {
		return nil, locales.AiKeyMissing()
	}
	return &ai.Client{BaseURL: p.BaseURL, Key: st.key, Model: m.Name, MaxTokens: p.MaxTokens}, ""
}

// aiToolsEnabled says whether the assistant may use tools on the session:
// it is connected, and the options allow commands.
func (a *App) aiToolsEnabled(s *Session) bool {
	return a.settings.AI.ExecTool() && s != nil && (s.client != nil || s.isADB())
}

// aiAsk sends prompt as the next question of s's conversation.
func (a *App) aiAsk(s *Session, prompt string) {
	if s == nil || strings.TrimSpace(prompt) == "" {
		return
	}
	st := &s.ai
	if !a.settings.AI.Enabled {
		st.err = locales.AiEnableFirst()
		return
	}
	client, why := a.aiClient()
	if client == nil {
		st.err = why
		a.redraw()
		return
	}
	st.err = ""
	st.turns = append(st.turns, aiTurn{prompt: prompt, model: a.settings.AI.ModelLabel()})
	st.toolMsgs, st.callQueue, st.pending = nil, nil, nil
	a.aiRequest(s)
}

// aiRequest sends the messages of s's conversation so far, and reads the
// answer out there.
func (a *App) aiRequest(s *Session) {
	st := &s.ai
	client, why := a.aiClient()
	if client == nil {
		st.err = why
		a.redraw()
		return
	}
	if st.cancel != nil {
		st.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.settings.AI.Timeout())
	st.cancel = cancel
	st.busy = true
	st.seq++
	a.redraw()
	go a.aiRead(ctx, s, st.seq, client, a.aiMessages(s), a.aiTools(s))
}

// aiRead reads the answer of one request, posting its pieces as they come
// and its end when it ends.
func (a *App) aiRead(ctx context.Context, s *Session, seq int, client *ai.Client, msgs []ai.Message, tools []ai.Tool) {
	var text, reasoning strings.Builder
	last := time.Now()
	flush := func() {
		if text.Len() == 0 && reasoning.Len() == 0 {
			return
		}
		t, r := text.String(), reasoning.String()
		text.Reset()
		reasoning.Reset()
		a.post(func() { a.aiAppend(s, seq, t, r) })
	}
	res, err := client.Chat(ctx, msgs, tools, func(t, r string) {
		text.WriteString(t)
		reasoning.WriteString(r)
		if time.Since(last) >= aiFlush {
			last = time.Now()
			flush()
		}
	})
	flush()
	a.post(func() { a.aiDone(s, seq, res, err) })
}

// aiAppend puts the pieces of an answer in its turn, unless another request
// has taken its place.
func (a *App) aiAppend(s *Session, seq int, text, reasoning string) {
	st := &s.ai
	if seq != st.seq || len(st.turns) == 0 {
		return
	}
	t := &st.turns[len(st.turns)-1]
	t.answer += text
	t.reasoning += reasoning
}

// aiDone ends the answer of a request: its tools run, or the turn stays as
// it is.
func (a *App) aiDone(s *Session, seq int, res ai.Result, err error) {
	st := &s.ai
	if seq != st.seq || len(st.turns) == 0 {
		return
	}
	st.busy = false
	t := &st.turns[len(st.turns)-1]
	if err != nil {
		switch {
		case errors.Is(err, ai.ErrStopped):
			t.stopped = true
		case errors.Is(err, ai.ErrTimeout):
			t.failed = locales.AiTimeoutErr()
		default:
			t.failed = err.Error()
		}
		if st.cancel != nil {
			st.cancel()
			st.cancel = nil
		}
		a.redraw()
		return
	}
	if res.Content != "" && t.answer == "" {
		t.answer = res.Content
	}
	if len(res.Calls) == 0 {
		a.redraw()
		return
	}
	// The assistant asked for tools: its call goes in the conversation,
	// and the results follow it.
	st.toolMsgs = append(st.toolMsgs, ai.Message{Role: "assistant", Content: res.Content, ToolCalls: res.Calls})
	if t.tools >= aiMaxToolRounds {
		for _, call := range res.Calls {
			a.aiToolResult(s, call, locales.AiTooManyTools())
		}
		a.aiContinue(s)
		return
	}
	st.callQueue = res.Calls
	a.aiNextCall(s)
}

// aiNextCall runs the next tool call of the answer, or asks for the
// answer that follows the results.
func (a *App) aiNextCall(s *Session) {
	st := &s.ai
	if len(st.callQueue) == 0 {
		a.aiContinue(s)
		return
	}
	call := st.callQueue[0]
	st.callQueue = st.callQueue[1:]
	switch call.Function.Name {
	case "exec_command":
		cmd := aiArgument(call, "command")
		if cmd == "" {
			a.aiToolResult(s, call, locales.AiEmptyCommand())
			a.aiNextCall(s)
			return
		}
		if !a.settings.AI.ExecTool() {
			a.aiToolResult(s, call, locales.AiToolsOff())
			a.aiNextCall(s)
			return
		}
		if a.settings.AI.ConfirmExec() {
			st.pending = &aiPending{call: call, cmd: cmd}
			a.redraw()
			return
		}
		a.aiExec(s, call, cmd)
	case "get_monitor_snapshot":
		a.aiToolResult(s, call, a.aiMonitorText(s))
		a.aiNextCall(s)
	case "list_remote_dir":
		a.aiToolResult(s, call, a.aiRemoteDirText(s))
		a.aiNextCall(s)
	default:
		a.aiToolResult(s, call, locales.AiUnknownTool(call.Function.Name))
		a.aiNextCall(s)
	}
}

// aiExec runs cmd on s's host, on a channel of its own, and goes on with
// the conversation when it ends.
func (a *App) aiExec(s *Session, call ai.ToolCall, cmd string) {
	if s == nil || (s.client == nil && !s.isADB()) {
		a.aiToolResult(s, call, locales.AiNoSession())
		a.aiNextCall(s)
		return
	}
	seq := s.ai.seq
	a.runOnSession(s, cmd, func(out string, err error) {
		if seq != s.ai.seq {
			return
		}
		if err != nil {
			a.aiToolResult(s, call, locales.AiExecFailed(err.Error()))
		} else {
			a.aiToolResult(s, call, out)
		}
		a.aiNextCall(s)
	})
}

// aiToolResult puts what a tool answered in the conversation, as the model
// reads it.
func (a *App) aiToolResult(s *Session, call ai.ToolCall, text string) {
	if s == nil {
		return
	}
	st := &s.ai
	text = strings.TrimSpace(text)
	if text == "" {
		text = "(no output)"
	}
	if len(text) > aiToolChars {
		text = text[:aiToolChars] + "\n…（输出过长已截断）"
	}
	st.toolMsgs = append(st.toolMsgs, ai.Message{Role: "tool", ToolCallID: call.ID, Content: text})
	if len(st.turns) > 0 {
		st.turns[len(st.turns)-1].tools++
	}
}

// aiContinue asks for the answer that follows the tool results.
func (a *App) aiContinue(s *Session) {
	a.aiRequest(s)
}

// aiAllow runs the command the assistant waits for.
func (a *App) aiAllow(s *Session) {
	if s == nil || s.ai.pending == nil {
		return
	}
	st := &s.ai
	p := st.pending
	st.pending = nil
	a.aiExec(s, p.call, p.cmd)
}

// aiDeny tells the assistant the user would not run its command.
func (a *App) aiDeny(s *Session) {
	if s == nil || s.ai.pending == nil {
		return
	}
	st := &s.ai
	p := st.pending
	st.pending = nil
	a.aiToolResult(s, p.call, locales.AiDenied())
	a.aiNextCall(s)
}

// aiStop stops the request out there.
func (a *App) aiStop(s *Session) {
	if s == nil {
		return
	}
	st := &s.ai
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.seq++ // what it answers from now on is dropped
	st.busy = false
	st.pending = nil
	st.callQueue = nil
	if len(st.turns) > 0 {
		st.turns[len(st.turns)-1].stopped = true
	}
	a.redraw()
}

// aiClear starts a new conversation on s.
func (a *App) aiClear(s *Session) {
	if s == nil {
		return
	}
	st := &s.ai
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.seq++
	st.turns, st.toolMsgs, st.callQueue, st.pending = nil, nil, nil, nil
	st.busy, st.err, st.input = false, "", ""
	st.caret, st.lastInput, st.caretSet, st.newCaret = 0, "", false, 0
	a.redraw()
}

// aiRegenerate asks the last question of s again.
func (a *App) aiRegenerate(s *Session) {
	if s == nil || s.ai.busy || len(s.ai.turns) == 0 {
		return
	}
	st := &s.ai
	prompt := st.turns[len(st.turns)-1].prompt
	st.turns = st.turns[:len(st.turns)-1]
	a.aiAsk(s, prompt)
}

// aiSend sends what the user typed on s's page.
func (a *App) aiSend(s *Session) {
	if s == nil {
		return
	}
	st := &s.ai
	prompt := strings.TrimSpace(st.input)
	if prompt == "" || st.busy {
		return
	}
	st.input = ""
	a.aiAsk(s, prompt)
}

// aiMessages is s's conversation as the service reads it: the instructions,
// the turns that answered, and the turn being answered with the tool calls
// and their results.
func (a *App) aiMessages(s *Session) []ai.Message {
	if s == nil {
		return nil
	}
	st := &s.ai
	var msgs []ai.Message
	if sys := a.aiSystem(s); sys != "" {
		msgs = append(msgs, ai.Message{Role: "system", Content: sys})
	}
	start := len(st.turns) - aiMaxTurns
	if start < 0 {
		start = 0
	}
	context := a.aiTerminalText(s)
	for i := start; i < len(st.turns); i++ {
		t := &st.turns[i]
		if strings.TrimSpace(t.prompt) == "" {
			continue
		}
		content := t.prompt
		if i == start && context != "" {
			content += "\n\n---\n" + context
		}
		msgs = append(msgs, ai.Message{Role: "user", Content: content})
		if i == len(st.turns)-1 {
			msgs = append(msgs, st.toolMsgs...)
			continue
		}
		if t.answer == "" || t.stopped {
			continue // not a turn to carry on from
		}
		msgs = append(msgs, ai.Message{Role: "assistant", Content: t.answer})
	}
	return msgs
}

// aiSystem is what the assistant is told before s's conversation: its
// instructions, the terminal's screen if the options say so, and what it
// may do on the host.
func (a *App) aiSystem(s *Session) string {
	cfg := a.settings.AI
	sys := cfg.SystemPrompt
	if strings.TrimSpace(sys) == "" {
		sys = locales.AiDefaultSystemPrompt() // in the language of the window
	}
	if cfg.ScreenContext {
		if text := a.aiTerminalText(s); text != "" {
			sys += "\n\n[终端当前屏幕内容]\n" + text
		}
	}
	if s != nil && (s.client != nil || s.isADB()) && cfg.ExecTool() {
		host := s.label()
		sys += "\n\n[当前环境] 用户已通过 SSH 连接主机「" + host + "」。" +
			"你可以调用 exec_command 工具在该主机上执行命令来获取真实信息（系统版本、内核、资源占用等）。" +
			"另有 get_monitor_snapshot（应用正在轮询的监控快照：CPU/内存/磁盘/负载）和 list_remote_dir（文件面板当前目录）两个只读工具。" +
			"凡是涉及「当前系统 / 主机是什么」这类事实性问题，先调用工具获取输出，再依据输出回答，不要凭空猜测。" +
			"命令在独立通道执行、不会出现在用户终端里。优先使用只读命令，禁止执行删除、格式化等破坏性操作。"
	}
	return sys
}

// aiTerminalText is the end of what s's terminal shows, as many lines as the
// options say.
func (a *App) aiTerminalText(s *Session) string {
	if s == nil || s.term() == nil {
		return ""
	}
	text := strings.TrimRight(s.term().Text(), "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if limit := a.settings.AI.ContextLineCount(); len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}

// aiTools are the tools the model may call, when s is connected and the
// options allow commands.
func (a *App) aiTools(s *Session) []ai.Tool {
	if !a.aiToolsEnabled(s) {
		return nil
	}
	return []ai.Tool{
		{Type: "function", Function: ai.ToolFunc{
			Name: "exec_command",
			Description: "在用户当前 SSH 连接的远程主机上执行一条 shell 命令并返回输出。" +
				"当需要获知主机的真实状态（系统版本、内核、硬件资源、磁盘、进程、文件内容等）时调用本工具，基于真实输出回答，不要凭空猜测。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type": "string",
						"description": "要执行的 shell 命令。必须是单条非交互命令（避免 vim/top 等需要交互的程序），" +
							"例如 uname -a、df -h、cat /etc/os-release。",
					},
				},
				"required": []string{"command"},
			},
		}},
		{Type: "function", Function: ai.ToolFunc{
			Name: "get_monitor_snapshot",
			Description: "获取当前主机的最新监控快照：CPU 使用率、系统负载、内存/交换、网络速率、各挂载点磁盘占用。" +
				"这是应用自身正在轮询的数据，秒回且不占用远端 shell。回答「这台机器是否吃紧」「磁盘是不是要满了」时优先调用它。",
			Parameters: map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		}},
		{Type: "function", Function: ai.ToolFunc{
			Name:        "list_remote_dir",
			Description: "列出 SFTP 文件面板当前目录的条目（名称、类型、大小、修改时间）。只读，且不会切换用户正在浏览的目录。需要查看其它路径时改用 exec_command 执行 ls。",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
		}},
	}
}

// aiArgument is an argument of a tool call, as the model wrote it in JSON.
func aiArgument(call ai.ToolCall, name string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return ""
	}
	s, _ := args[name].(string)
	return strings.TrimSpace(s)
}

// aiMonitorText is s's monitor snapshot as the model reads it.
func (a *App) aiMonitorText(s *Session) string {
	if s == nil {
		return locales.AiNoSession()
	}
	st := s.stats
	if !st.OK && st.MemTotal == 0 {
		return locales.AiNoMonitor()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "主机：%s\n", s.label())
	fmt.Fprintf(&b, "运行时长：%s\n", st.Uptime.Round(time.Second))
	fmt.Fprintf(&b, "负载（1/5/15 分钟）：%.2f / %.2f / %.2f\n", st.Load[0], st.Load[1], st.Load[2])
	fmt.Fprintf(&b, "CPU 使用率：%.0f%%\n", st.CPU)
	fmt.Fprintf(&b, "内存：%s / %s\n", aiBytes(int64(st.MemUsed)), aiBytes(int64(st.MemTotal)))
	fmt.Fprintf(&b, "交换：%s / %s\n", aiBytes(int64(st.SwapUsed)), aiBytes(int64(st.SwapTotal)))
	if st.Iface != "" {
		fmt.Fprintf(&b, "网络 %s：↓%.1f KB/s ↑%.1f KB/s\n", st.Iface, st.RxRate/1024, st.TxRate/1024)
	}
	for _, d := range st.Disks {
		fmt.Fprintf(&b, "磁盘 %s：可用 %s / %s\n", d.Mount, aiBytes(int64(d.Avail)), aiBytes(int64(d.Size)))
	}
	fmt.Fprintf(&b, "进程数：%d，登录用户：%d\n", st.ProcTotal, st.Users)
	return strings.TrimSpace(b.String())
}

// aiRemoteDirText is s's file panel current directory as the model reads
// it.
func (a *App) aiRemoteDirText(s *Session) string {
	if s == nil {
		return locales.AiNoSession()
	}
	f := &s.files
	if f.cwd == "" {
		return locales.AiNoDir()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "目录：%s\n", f.cwd)
	for _, e := range f.entries {
		kind := "文件"
		if e.IsDir() {
			kind = "目录"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", e.Name, kind, aiBytes(e.Size), e.ModTime.Format("2006-01-02 15:04"))
	}
	if len(f.entries) == 0 {
		b.WriteString("（空目录）\n")
	}
	return strings.TrimSpace(b.String())
}

// aiBytes is a size as the model reads it.
func aiBytes(n int64) string {
	if n < 0 {
		return "0 B"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 3; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
