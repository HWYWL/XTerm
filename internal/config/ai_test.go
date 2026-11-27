package config

import (
	"net/url"
	"testing"
)

// The services and models the assistant is offered by default: each
// provider is distinct, has an address the client can post to, and has a
// model to ask, and each model names its provider.
func TestDefaultAISettings(t *testing.T) {
	providers := DefaultAIProviders()
	if len(providers) < 15 {
		t.Fatalf("%d providers, want more", len(providers))
	}
	ids := make(map[string]bool, len(providers))
	names := make(map[string]bool, len(providers))
	for _, p := range providers {
		if p.ID == "" || p.Name == "" {
			t.Errorf("provider %+v", p)
		}
		if ids[p.ID] {
			t.Errorf("provider %q is listed twice", p.ID)
		}
		ids[p.ID] = true
		if names[p.Name] {
			t.Errorf("the name %q is used twice", p.Name)
		}
		names[p.Name] = true
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			t.Errorf("provider %q at %q", p.ID, p.BaseURL)
		}
	}
	models := DefaultAIModels()
	modelIDs := make(map[string]bool, len(models))
	ofProvider := make(map[string]int, len(providers))
	for _, m := range models {
		if modelIDs[m.ID] {
			t.Errorf("model %q is listed twice", m.ID)
		}
		modelIDs[m.ID] = true
		if !ids[m.Provider] {
			t.Errorf("model %q belongs to no provider", m.ID)
		}
		if want := m.Provider + ":" + m.Name; m.ID != want {
			t.Errorf("model ID %q, want %q", m.ID, want)
		}
		ofProvider[m.Provider]++
	}
	for _, p := range providers {
		if ofProvider[p.ID] == 0 {
			t.Errorf("provider %q has no model", p.ID)
		}
	}
}

// A file without the assistant's settings reads the defaults, and asks the
// first provider's first model.
func TestDefaultAISettingsChoose(t *testing.T) {
	s := DefaultAISettings()
	p, ok := s.ChosenProvider()
	if !ok || p.ID != "deepseek" {
		t.Fatalf("chosen provider %+v", p)
	}
	m, ok := s.ChosenModel()
	if !ok || m.Provider != p.ID {
		t.Errorf("chosen model %+v", m)
	}
	// Anthropic's compatibility layer wants the answer's length said, so
	// its provider carries one; the others leave it to the service.
	if p := DefaultAIProviders(); p[2].ID != "anthropic" || p[2].MaxTokens <= 0 {
		t.Errorf("Anthropic's provider %+v", p[2])
	}
}

// A file saved before a service was added gains it on the next read, while
// the services it lists keep their order and their own addresses, and one
// the user deleted stays deleted once the file says which defaults it has.
func TestAISettingsGainNewDefaults(t *testing.T) {
	old := AISettings{
		Providers: []AIProvider{
			{ID: "mine", Name: "Mine", BaseURL: "https://llm.example.com/v1"},
			{ID: "deepseek", Name: "DeepSeek", BaseURL: "https://proxy.example.com/v1"},
		},
		Models: []AIModel{{ID: "deepseek:deepseek-v4-flash", Provider: "deepseek", Name: "deepseek-v4-flash"}},
	}
	old.fill()
	if old.Providers[0].ID != "mine" || old.Providers[1].BaseURL != "https://proxy.example.com/v1" {
		t.Errorf("the list the user edited %+v", old.Providers[:2])
	}
	if _, ok := old.ProviderByID("anthropic"); !ok {
		t.Error("a service added since was not put in")
	}
	if _, ok := old.ModelByID("anthropic:claude-sonnet-4-6"); !ok {
		t.Error("a model added since was not put in")
	}
	if old.DefaultsVersion != aiDefaultsVersion {
		t.Errorf("the file says defaults %d", old.DefaultsVersion)
	}

	deleted := AISettings{
		DefaultsVersion: aiDefaultsVersion,
		Providers:       []AIProvider{{ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"}},
		Models:          []AIModel{},
	}
	deleted.fill()
	if _, ok := deleted.ProviderByID("anthropic"); ok {
		t.Error("a service the user deleted came back")
	}
	if len(deleted.Providers) != 1 {
		t.Errorf("the list %+v", deleted.Providers)
	}
}
