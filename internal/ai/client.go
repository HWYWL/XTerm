// Package ai talks to an OpenAI-compatible chat service: it sends the
// messages of a conversation and reads the answer as it streams, with the
// tool calls the model asks for.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Message is one turn of a conversation, as the service reads it.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is what the model asked a tool to do.
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function FuncCall `json:"function"`
}

// FuncCall is a tool call's name and its arguments, as JSON.
type FuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is a tool the model may call, as the service reads it.
type Tool struct {
	Type     string   `json:"type"`
	Function ToolFunc `json:"function"`
}

// ToolFunc is a tool's description and the schema of its arguments.
type ToolFunc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// Result is what one request answered: the text, what the model thought
// before it, and the tool calls it asked for.
type Result struct {
	Content   string
	Reasoning string
	Calls     []ToolCall
}

// Client talks to one service and model.
type Client struct {
	// BaseURL is the service's root, as "https://api.deepseek.com/v1": the
	// request goes to its /chat/completions.
	BaseURL string
	// Key is the API key, sent as a bearer token; a service on this
	// machine needs none.
	Key string
	// Model is the name the service knows the model by.
	Model string
	// MaxTokens is the longest answer asked for, where the service wants
	// it said: Anthropic's compatibility layer does. Zero leaves it to
	// the service's own default.
	MaxTokens int
	// HTTP is the client to use; nil is http.DefaultClient.
	HTTP *http.Client
}

// FirstIdle is how long the service may take to start answering, and
// TailIdle how long it may pause once it has. A model that thinks longer
// than FirstIdle is cut off: a silent stream is a lost one.
const (
	FirstIdle = 30 * time.Second
	TailIdle  = 6 * time.Second
)

// ErrStopped says the request was cancelled before it answered.
var ErrStopped = errors.New("the request was stopped")

// ErrTimeout says the service took too long: it did not start answering, or
// paused as it streamed.
var ErrTimeout = errors.New("the model took too long to answer")

// Chat sends msgs and reads the answer, calling onDelta with each piece of
// text and of the model's reasoning as it arrives. It returns the whole
// answer, or the tool calls the model asked for instead of one.
func (c *Client) Chat(ctx context.Context, msgs []Message, tools []Tool, onDelta func(text, reasoning string)) (Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	payload := map[string]any{"model": c.Model, "messages": msgs, "stream": true}
	if c.MaxTokens > 0 {
		payload["max_tokens"] = c.MaxTokens
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ErrStopped
		}
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return Result{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, oneLine(string(msg)))
	}

	var timedOut atomic.Bool
	idle := time.AfterFunc(FirstIdle, func() { timedOut.Store(true); cancel() })
	defer idle.Stop()

	result, err := c.readStream(resp.Body, onDelta, idle)
	if err != nil && timedOut.Load() {
		return result, ErrTimeout
	}
	if err != nil && ctx.Err() != nil {
		return result, ErrStopped
	}
	return result, err
}

// Test sends one short message without streaming, to find out whether the
// service answers with this key at all.
func (c *Client) Test(ctx context.Context) error {
	payload := map[string]any{
		"model":    c.Model,
		"messages": []Message{{Role: "user", Content: "ping"}},
		"stream":   false,
	}
	if c.MaxTokens > 0 {
		payload["max_tokens"] = c.MaxTokens
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, oneLine(string(data)))
	}
	if _, ok := answerOf(data); !ok {
		return errors.New("the service answered without a message")
	}
	return nil
}

// readStream reads the server-sent events of resp, and the whole body of a
// service that ignored the stream and answered once.
func (c *Client) readStream(body io.Reader, onDelta func(text, reasoning string), idle *time.Timer) (Result, error) {
	var text, reasoning strings.Builder
	calls := map[int]*ToolCall{}
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		idle.Reset(TailIdle)
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var ch chunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			continue // a keep-alive or a comment line
		}
		if ch.Error != nil && ch.Error.Message != "" {
			return Result{}, errors.New(oneLine(ch.Error.Message))
		}
		for _, choice := range ch.Choices {
			d := choice.Delta
			if d.Content != "" {
				text.WriteString(d.Content)
				if onDelta != nil {
					onDelta(d.Content, "")
				}
			}
			if d.Reasoning != "" {
				reasoning.WriteString(d.Reasoning)
				if onDelta != nil {
					onDelta("", d.Reasoning)
				}
			}
			for _, tc := range d.ToolCalls {
				call := calls[tc.Index]
				if call == nil {
					call = &ToolCall{Type: "function"}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" && call.Function.Name == "" {
					call.Function.Name = tc.Function.Name
				}
				call.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Result{}, err
	}
	if text.Len() == 0 && len(calls) == 0 {
		// A service that ignored stream answered one JSON object.
		if data, err := io.ReadAll(body); err == nil {
			if full, ok := answerOf(data); ok {
				if onDelta != nil && full.Content != "" {
					onDelta(full.Content, "")
				}
				return full, nil
			}
		}
		return Result{}, errors.New("the service answered nothing")
	}
	result := Result{Content: text.String(), Reasoning: reasoning.String()}
	return result.withCalls(calls), nil
}

// withCalls puts the tool calls the stream asked for in order.
func (r Result) withCalls(calls map[int]*ToolCall) Result {
	if len(calls) == 0 {
		return r
	}
	indexes := make([]int, 0, len(calls))
	for i := range calls {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		r.Calls = append(r.Calls, *calls[i])
	}
	return r
}

// answerOf reads a whole answer, as a service that does not stream sends.
func answerOf(data []byte) (Result, bool) {
	var full struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				Reasoning string     `json:"reasoning_content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &full); err != nil || len(full.Choices) == 0 {
		return Result{}, false
	}
	m := full.Choices[0].Message
	return Result{Content: m.Content, Reasoning: m.Reasoning, Calls: m.ToolCalls}, true
}

// chunk is one server-sent event of an answer as it streams.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning_content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// oneLine is s as one line, the errors of a service being short enough.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
