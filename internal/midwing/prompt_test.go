package midwing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type inaccessibleVault struct{}

func (inaccessibleVault) Get(string) (string, error) { panic("prompt must not read credentials") }
func (inaccessibleVault) Set(string, string) error   { return errors.New("unavailable") }
func (inaccessibleVault) Delete(string) error        { return errors.New("unavailable") }
func TestPromptExamplesMatchAllowlist(t *testing.T) {
	for _, service := range services {
		for _, ep := range service.Endpoints {
			for _, path := range ep.Paths {
				path = strings.NewReplacer("OWNER", "openai", "REPO", "codex", "RECORD_ID", "123456789012345").Replace(path)
				permission, _, e := service.allowed("GET", path)
				if e != nil || permission != ep.Permission {
					t.Fatalf("invalid handoff example %s: %v", path, e)
				}
			}
		}
	}
}
func TestPromptIndependentOfSettings(t *testing.T) {
	s := testStore(t)
	if e := os.WriteFile(filepath.Join(s.Dir, "settings.json"), []byte(`broken`), 0600); e != nil {
		t.Fatal(e)
	}
	if prompt, e := s.AgentPrompt(); e != nil || prompt == "" {
		t.Fatal("generic prompt depended on configuration")
	}
	if _, e := s.RegistrySnapshot(); e == nil {
		t.Fatal("broken settings accepted by discovery")
	}
}
func TestActivityTimeJSONName(t *testing.T) {
	b, e := json.Marshal(Activity{})
	if e != nil || !strings.Contains(string(b), `"time"`) {
		t.Fatal("activity time field must be stable")
	}
}

func TestAgentPromptIsServiceNeutral(t *testing.T) {
	registerPasswordFixture(t)
	s := testStore(t)
	before, e := s.AgentPrompt()
	if e != nil {
		t.Fatal(e)
	}
	s.SaveService("sample-api", "sample-api-secret", []string{"profile"})
	seedPasswordConnection(t, s, "sample-auth", "sample-secret", []string{"charges"})
	after, e := s.AgentPrompt()
	if e != nil || before != after {
		t.Fatal("prompt changed with connected accounts")
	}
	for _, name := range []string{"sample-auth", "sample-api", "sample-api-secret", "sample-secret"} {
		if strings.Contains(strings.ToLower(after), name) {
			t.Fatalf("service-specific content in prompt: %s", name)
		}
	}
	if !strings.Contains(after, "midwing services") {
		t.Fatal("prompt cannot discover enabled operations")
	}
}
