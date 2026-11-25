package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// The AI assistant: Options › AI, and the AI page of the left column. Its
// settings go with the rest of the settings, in this device's file. The API
// keys go to the credential store, by AIKeyName, as the passwords do.

// AIProvider is an OpenAI-compatible service the assistant talks to. Its
// key is in the credential store, not here.
type AIProvider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
}

// AIModel is a model of a provider: the name the service knows it by.
type AIModel struct {
	ID       string `json:"id"`       // "<provider id>:<name>"
	Provider string `json:"provider"` // the provider's ID
	Name     string `json:"name"`
}

// AISettings is Options › AI: where the assistant talks to, what it says,
// and what it may do on the session's host.
type AISettings struct {
	// Enabled turns the assistant on: while it is off, the AI page asks
	// for it to be turned on and no request goes out.
	Enabled bool `json:"enabled,omitempty"`
	// Provider and Model are the chosen provider's ID and model's ID. An
	// empty one is the first of the list, as a file without them reads.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// ContextLines is how many lines of the terminal the first question
	// carries, at most. Zero is DefaultAIContextLines.
	ContextLines int `json:"contextLines,omitempty"`
	// TimeoutSeconds is how long a request may take. Zero is
	// DefaultAITimeoutSeconds.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// SystemPrompt is what the assistant is told before the questions.
	// An empty one is DefaultAISystemPrompt.
	SystemPrompt string `json:"systemPrompt,omitempty"`
	// NoExecTool keeps the assistant from running commands on the host at
	// all, when true; by default it may.
	NoExecTool bool `json:"noExecTool,omitempty"`
	// NoExecConfirm runs the commands the assistant asks for without
	// asking first, when true; by default each one waits for the user.
	NoExecConfirm bool `json:"noExecConfirm,omitempty"`
	// ScreenContext puts the terminal's screen in the assistant's
	// instructions, on every question, rather than only the first one's
	// context.
	ScreenContext bool `json:"screenContext,omitempty"`
	// Providers and Models are the services and their models the Options
	// page lists. A file without them reads the defaults, so that a list
	// the user emptied stays empty.
	Providers []AIProvider `json:"providers,omitempty"`
	Models    []AIModel    `json:"models,omitempty"`
}

// DefaultAIContextLines is how many lines of the terminal a question
// carries by default.
const DefaultAIContextLines = 200

// DefaultAITimeoutSeconds is how long a request may take by default.
const DefaultAITimeoutSeconds = 60

// AIKeyName is the credential store's name for a provider's API key.
func AIKeyName(providerID string) string { return "ai:" + providerID }

// DefaultAIProviders are the services the assistant knows by default, each
// with the API key to be filled in.
func DefaultAIProviders() []AIProvider {
	return []AIProvider{
		{ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1"},
		{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		{ID: "kimi", Name: "Kimi", BaseURL: "https://api.moonshot.cn/v1"},
		{ID: "glm", Name: "GLM", BaseURL: "https://open.bigmodel.cn/api/paas/v4"},
		{ID: "siliconflow", Name: "SiliconFlow", BaseURL: "https://api.siliconflow.cn/v1"},
		{ID: "ollama", Name: "Ollama", BaseURL: "http://127.0.0.1:11434/v1"},
	}
}

// DefaultAIModels are the models the assistant knows by default.
func DefaultAIModels() []AIModel {
	return []AIModel{
		{ID: "deepseek:deepseek-v4-flash", Provider: "deepseek", Name: "deepseek-v4-flash"},
		{ID: "deepseek:deepseek-v4-pro", Provider: "deepseek", Name: "deepseek-v4-pro"},
		{ID: "openai:gpt-4o-mini", Provider: "openai", Name: "gpt-4o-mini"},
		{ID: "kimi:kimi-k2-turbo-preview", Provider: "kimi", Name: "kimi-k2-turbo-preview"},
		{ID: "glm:glm-4-flash", Provider: "glm", Name: "glm-4-flash"},
		{ID: "siliconflow:Qwen/Qwen3-8B", Provider: "siliconflow", Name: "Qwen/Qwen3-8B"},
		{ID: "ollama:qwen2.5:7b-instruct", Provider: "ollama", Name: "qwen2.5:7b-instruct"},
	}
}

// DefaultAISettings is what a file without the assistant's settings reads.
func DefaultAISettings() AISettings {
	var s AISettings
	s.fill()
	return s
}

// fill gives a file without the lists the defaults.
func (s *AISettings) fill() {
	if s.Providers == nil {
		s.Providers = DefaultAIProviders()
	}
	if s.Models == nil {
		s.Models = DefaultAIModels()
	}
}

// ProviderByID is the provider with that ID, if it is in the list.
func (s AISettings) ProviderByID(id string) (AIProvider, bool) {
	for _, p := range s.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return AIProvider{}, false
}

// ChosenProvider is the provider the assistant talks to: the chosen one, or
// the first of the list.
func (s AISettings) ChosenProvider() (AIProvider, bool) {
	if p, ok := s.ProviderByID(s.Provider); ok {
		return p, true
	}
	if len(s.Providers) == 0 {
		return AIProvider{}, false
	}
	return s.Providers[0], true
}

// ModelsOf are the models of the provider with that ID.
func (s AISettings) ModelsOf(providerID string) []AIModel {
	var out []AIModel
	for _, m := range s.Models {
		if m.Provider == providerID {
			out = append(out, m)
		}
	}
	return out
}

// ChosenModel is the model the assistant asks: the chosen one of the chosen
// provider, or the first of the provider's.
func (s AISettings) ChosenModel() (AIModel, bool) {
	if m, ok := s.ModelByID(s.Model); ok {
		return m, true
	}
	p, ok := s.ChosenProvider()
	if !ok {
		return AIModel{}, false
	}
	if models := s.ModelsOf(p.ID); len(models) > 0 {
		return models[0], true
	}
	return AIModel{}, false
}

// ModelByID is the model with that ID, if it is in the list.
func (s AISettings) ModelByID(id string) (AIModel, bool) {
	for _, m := range s.Models {
		if m.ID == id {
			return m, true
		}
	}
	return AIModel{}, false
}

// ModelLabel is the chosen provider and model, as the page shows them.
func (s AISettings) ModelLabel() string {
	p, ok := s.ChosenProvider()
	if !ok {
		return ""
	}
	m, ok := s.ChosenModel()
	if !ok {
		return p.Name
	}
	return p.Name + " · " + m.Name
}

// ContextLineCount is ContextLines, or the default.
func (s AISettings) ContextLineCount() int {
	if s.ContextLines <= 0 {
		return DefaultAIContextLines
	}
	return s.ContextLines
}

// Timeout is how long a request may take.
func (s AISettings) Timeout() time.Duration {
	sec := s.TimeoutSeconds
	if sec <= 0 {
		sec = DefaultAITimeoutSeconds
	}
	return time.Duration(sec) * time.Second
}

// ExecTool says whether the assistant may run commands on the host.
func (s AISettings) ExecTool() bool { return !s.NoExecTool }

// ConfirmExec says whether each command waits for the user.
func (s AISettings) ConfirmExec() bool { return !s.NoExecConfirm }

// AddProvider adds a provider of the user's own, and chooses it.
func (s *AISettings) AddProvider(name, baseURL string) string {
	id := fmt.Sprintf("custom-%d", time.Now().Unix())
	s.Providers = append(s.Providers, AIProvider{ID: id, Name: name, BaseURL: baseURL})
	s.Provider = id
	return id
}

// DeleteProvider removes a provider and its models, choosing another.
func (s *AISettings) DeleteProvider(id string) {
	var kept []AIProvider
	for _, p := range s.Providers {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(s.Providers) {
		return
	}
	s.Providers = kept
	var models []AIModel
	for _, m := range s.Models {
		if m.Provider != id {
			models = append(models, m)
		}
	}
	s.Models = models
	if s.Provider == id {
		s.Provider = ""
	}
	if m, ok := s.ModelByID(s.Model); !ok || m.Provider != s.Provider {
		s.Model = ""
	}
}

// SetProviderURL changes the address of a provider.
func (s *AISettings) SetProviderURL(id, url string) {
	for i := range s.Providers {
		if s.Providers[i].ID == id {
			s.Providers[i].BaseURL = url
		}
	}
}

// AddModel adds a model of a provider, and chooses it.
func (s *AISettings) AddModel(providerID, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	id := providerID + ":" + name
	if _, ok := s.ModelByID(id); ok {
		s.Model = id
		return
	}
	s.Models = append(s.Models, AIModel{ID: id, Provider: providerID, Name: name})
	s.Model = id
}

// DeleteModel removes a model, choosing another of its provider.
func (s *AISettings) DeleteModel(id string) {
	var kept []AIModel
	for _, m := range s.Models {
		if m.ID != id {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(s.Models) {
		return
	}
	s.Models = kept
	if s.Model == id {
		s.Model = ""
	}
}

// LocalEndpoint says whether the provider is on this machine, where a key
// is not needed: a file without one reads localhost.
func LocalEndpoint(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0":
		return true
	}
	return false
}
