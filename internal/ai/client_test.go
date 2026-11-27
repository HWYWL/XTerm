package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer answers every request with lines, each as a server-sent event.
func sseServer(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("accept %q", got)
		}
		var req struct {
			Model    string    `json:"model"`
			Stream   bool      `json:"stream"`
			Messages []Message `json:"messages"`
			Tools    []Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("body: %v", err)
		}
		if req.Model != "m" || !req.Stream || len(req.Messages) == 0 {
			t.Errorf("request %+v", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range lines {
			fmt.Fprintf(w, "data: %s\n\n", line)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChatStreams(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`{"choices":[{"delta":{"content":"Hel"}}]}`,
		`{"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
		`[DONE]`,
	})
	var text, reasoning strings.Builder
	c := &Client{BaseURL: srv.URL + "/v1", Model: "m"}
	res, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil,
		func(dt, dr string) { text.WriteString(dt); reasoning.WriteString(dr) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Content != "Hello" || res.Reasoning != "think" {
		t.Errorf("answer %+v", res)
	}
	if text.String() != "Hello" || reasoning.String() != "think" {
		t.Errorf("streamed %q, %q", text.String(), reasoning.String())
	}
}

func TestChatToolCalls(t *testing.T) {
	srv := sseServer(t, []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"exec_command","arguments":"{\"comm"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"and\":\"df -h\"}"}}]}}]}`,
		`[DONE]`,
	})
	c := &Client{BaseURL: srv.URL + "/v1", Model: "m"}
	res, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "disk?"}},
		[]Tool{{Type: "function", Function: ToolFunc{Name: "exec_command"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Calls) != 1 {
		t.Fatalf("calls %+v", res.Calls)
	}
	call := res.Calls[0]
	if call.ID != "c1" || call.Function.Name != "exec_command" || call.Function.Arguments != `{"command":"df -h"}` {
		t.Errorf("call %+v", call)
	}
}

func TestChatHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", Model: "m"}
	_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("error %v", err)
	}
}

func TestChatStopped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // held open, as a model thinking
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{BaseURL: srv.URL + "/v1", Model: "m"}
	go func() { cancel() }()
	_, err := c.Chat(ctx, []Message{{Role: "user", Content: "hi"}}, nil, nil)
	if !errors.Is(err, ErrStopped) {
		t.Errorf("error %v", err)
	}
}

func TestTest(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			t.Error("streamed a test")
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"pong"}}]}`)
	}))
	defer ok.Close()
	c := &Client{BaseURL: ok.URL + "/v1", Model: "m"}
	if err := c.Test(context.Background()); err != nil {
		t.Errorf("test: %v", err)
	}
}

// The longest answer is asked for where the service wants it said, as
// Anthropic's compatibility layer does, and left out elsewhere.
func TestMaxTokens(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		bodies = append(bodies, m)
		if stream, _ := m["stream"].(bool); !stream {
			io.WriteString(w, `{"choices":[{"message":{"content":"pong"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v1", Model: "m", MaxTokens: 8192}
	if _, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := bodies[0]["max_tokens"]; got != float64(8192) {
		t.Errorf("the answer was capped at %v", got)
	}
	// The connection's test carries it too, as the service checks the same
	// fields.
	if err := c.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := bodies[1]["max_tokens"]; got != float64(8192) {
		t.Errorf("the test capped at %v", got)
	}
	c.MaxTokens = 0
	if err := c.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, ok := bodies[2]["max_tokens"]; ok {
		t.Errorf("a service without a cap is sent %v", got)
	}
}
