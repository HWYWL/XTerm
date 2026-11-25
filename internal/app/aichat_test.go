package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// aiServer answers the assistant with the pieces of one answer.
func aiServer(t *testing.T, pieces ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, p := range pieces {
			fmt.Fprintf(w, "data: %s\n\n", p)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// aiApp is an app whose assistant talks to the server at baseURL, its
// panel on the AI tab of the selected session.
func aiApp(t *testing.T, baseURL string) (*App, *fakePlatform, *ui.Tester, *Session) {
	t.Helper()
	a, p := testApp(t)
	a.settings.AI.Enabled = true
	a.settings.AI.Providers = []config.AIProvider{{ID: "test", Name: "Test", BaseURL: baseURL}}
	a.settings.AI.Models = []config.AIModel{{ID: "test:m", Provider: "test", Name: "m"}}
	a.settings.AI.Provider, a.settings.AI.Model = "test", "test:m"
	s := demoSession(a, "web-1", "127.0.0.1")
	a.active = 0
	a.bottomTab = tabAI
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	return a, p, tt, s
}

// The AI tab answers a question as the model streams it.
func TestAIChatAnswers(t *testing.T) {
	srv := aiServer(t,
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)
	a, p, tt, s := aiApp(t, srv.URL)
	if !tt.HasText(locales.AiEmpty()) {
		t.Fatalf("the empty page: %q", tt.Texts())
	}
	s.ai.input = "hi"
	a.aiSend(s)
	p.until(t, "the answer", func() bool {
		return !s.ai.busy && len(s.ai.turns) == 1 && s.ai.turns[0].answer == "Hello"
	})
	tt.Frame()
	if !tt.HasText("Hello") || !tt.HasText("hi") {
		t.Errorf("the page shows %q", tt.Texts())
	}
	saveShot(t, tt, "ai-chat-en.png")
}

// Each session has a conversation of its own.
func TestAIChatPerSession(t *testing.T) {
	srv := aiServer(t, `{"choices":[{"delta":{"content":"ok"}}]}`)
	a, p, tt, s := aiApp(t, srv.URL)
	other := demoSession(a, "db-1", "10.0.0.12")
	tt.Frame()
	s.ai.input = "hi"
	a.aiSend(s)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	if len(other.ai.turns) != 0 {
		t.Errorf("the other session has %d turns", len(other.ai.turns))
	}
	a.active = slices.Index(a.sessions, other)
	tt.Frame()
	if tt.HasText("hi") || !tt.HasText(locales.AiEmpty()) {
		t.Errorf("the other session's page shows %q", tt.Texts())
	}
}

// Enter sends the question, and Ctrl+Enter breaks the line at the caret.
func TestAIChatComposerKeys(t *testing.T) {
	srv := aiServer(t, `{"choices":[{"delta":{"content":"ok"}}]}`)
	_, p, tt, s := aiApp(t, srv.URL)
	if err := tt.Click(locales.AiAskPlaceholder()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	tt.Type("first")
	tt.Key(ui.Ctrl, ui.KeyEnter)
	tt.Type("second")
	tt.Frame()
	if want := "first\nsecond"; s.ai.input != want {
		t.Fatalf("input %q, want %q", s.ai.input, want)
	}
	tt.Key(0, ui.KeyEnter)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	if got := s.ai.turns[0].prompt; got != "first\nsecond" {
		t.Errorf("prompt %q", got)
	}
	if s.ai.input != "" {
		t.Errorf("the box after Enter: %q", s.ai.input)
	}
}

// The first question carries the end of the session's terminal.
func TestAIChatCarriesTheTerminal(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a, p, tt, s := aiApp(t, srv.URL)
	done := make(chan struct{})
	defer close(done)
	term, err := terminal.New(terminal.Options{Conn: idleConn{done}})
	if err != nil {
		t.Skip("no terminal:", err)
	}
	defer term.Close()
	s.setTerm(term)
	tt.Frame()
	term.Feed([]byte("the-terminal-line\r\n"))

	s.ai.input = "what is this?"
	a.aiSend(s)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	if !strings.Contains(body, "the-terminal-line") {
		t.Errorf("the request does not carry the terminal: %s", body)
	}
}

// Stopping an answer says the turn was stopped.
func TestAIChatStops(t *testing.T) {
	held := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"a"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-held:
		}
	}))
	defer srv.Close()
	defer close(held)
	a, p, _, s := aiApp(t, srv.URL)
	s.ai.input = "hi"
	a.aiSend(s)
	p.until(t, "the first piece", func() bool { return s.ai.turns[0].answer != "" })
	a.aiStop(s)
	if s.ai.busy || !s.ai.turns[0].stopped {
		t.Errorf("after stop: busy %v, turn %+v", s.ai.busy, s.ai.turns[0])
	}
}

// A new conversation drops the turns of the old one.
func TestAIChatClear(t *testing.T) {
	srv := aiServer(t, `{"choices":[{"delta":{"content":"ok"}}]}`)
	a, p, tt, s := aiApp(t, srv.URL)
	s.ai.input = "hi"
	a.aiSend(s)
	p.until(t, "the answer", func() bool { return !s.ai.busy && len(s.ai.turns) == 1 })
	a.aiClear(s)
	tt.Frame()
	if len(s.ai.turns) != 0 || !tt.HasText(locales.AiEmpty()) {
		t.Errorf("after clear: %d turns, %q", len(s.ai.turns), tt.Texts())
	}
}

// The options page sets where the assistant talks to, and whether it may
// run commands.
func TestAIOptionsPage(t *testing.T) {
	a, p := testApp(t)
	a.openOptions()
	a.optionsNav.Page = pageAI
	tt := p.show(t, WindowOptions)
	tt.SetSize(760, 1400) // the page is longer than the window
	tt.Frame()
	for _, want := range []string{locales.OptionsAI(), locales.AiEnable(), locales.AiService(), locales.AiModel(),
		locales.AiAllowExec(), locales.AiTestConnection()} {
		if !tt.HasText(want) {
			t.Errorf("no %q; texts %q", want, tt.Texts())
		}
	}
	allow := a.settings.AI.ExecTool()
	if err := tt.Click(locales.AiAllowExec()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	if a.settings.AI.ExecTool() == allow {
		t.Errorf("the toggle did not change: %v", a.settings.AI.ExecTool())
	}
	if !a.settings.AI.ConfirmExec() {
		t.Error("a command no longer waits for the user")
	}
	saveShot(t, tt, "options-ai-en.png")
}
