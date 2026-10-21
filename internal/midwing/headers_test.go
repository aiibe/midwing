package midwing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestStaticHeadersValidation(t *testing.T) {
	for _, name := range []string{
		"", "Bad Name", "Bad:Name", "Bad\rName", "Bad\nName", "Bad\x00Name", strings.Repeat("x", 81),
		"aUtHoRiZaTiOn", "x-api-KEY", "Proxy-Authorization", "Cookie", "Host", "Content-Length",
		"Connection", "Transfer-Encoding", "Keep-Alive", "Proxy-Connection", "TE", "Trailer", "Upgrade",
		"Expect", "HTTP2-Settings", "Accept", "Accept-Encoding", "Content-Type", "User-Agent",
	} {
		t.Run(name, func(t *testing.T) {
			d := tokenDefinition()
			d.Headers = StaticHeaders{name: "value"}
			if _, err := d.compile(); err == nil {
				t.Fatal("accepted invalid or reserved header")
			}
		})
	}
	for _, value := range []string{"line\rbreak", "line\nbreak", "nul\x00", "control\x1f", "delete\x7f"} {
		d := tokenDefinition()
		d.Headers = StaticHeaders{"X-Test": value}
		if _, err := d.compile(); err == nil {
			t.Errorf("accepted invalid value %q", value)
		}
	}
	d := tokenDefinition()
	d.Headers = StaticHeaders{"X-Test": "one", "x-test": "two"}
	if _, err := d.compile(); err == nil {
		t.Fatal("accepted duplicate header names")
	}
	d.Headers = StaticHeaders{"X-API-Version": "2026-08-14", "X-Empty": "", "X-Tab": "one\ttwo"}
	if _, err := d.compile(); err != nil {
		t.Fatalf("valid headers rejected: %v", err)
	}
}

func TestStaticHeadersJSON(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `"text"`, `{"X-Test":null}`, `{"X-Test":42}`, `{"X-Test":true}`, `{"X-Test":[]}`, `{"X-Test":{}}`, `{"X-Test":"a","x-test":"b"}`, `{"X-Test":"a","X-Test":"b"}`} {
		var headers StaticHeaders
		if err := json.Unmarshal([]byte(input), &headers); err == nil {
			t.Errorf("accepted invalid headers: %s", input)
		}
	}
}

func TestStaticHeadersDefinitionHash(t *testing.T) {
	d := tokenDefinition()
	legacyJSON := `{"id":"my-api","name":"My API","baseUrl":"https://example.test/v1","authKind":"token","header":"X-API-Key","prefix":"","endpoints":[{"path":"/items","queryParameters":["filter","limit"]},{"path":"/items/{id}","queryParameters":null}]}`
	legacyHash := sha256.Sum256([]byte(legacyJSON))
	if d.definitionHash() != hex.EncodeToString(legacyHash[:]) {
		t.Fatal("definition without headers changed its legacy hash")
	}
	d.Headers = StaticHeaders{}
	if d.definitionHash() != hex.EncodeToString(legacyHash[:]) {
		t.Fatal("empty headers changed the legacy hash")
	}
	d.Headers = StaticHeaders{"x-api-version": "2026-08-14"}
	hash := d.definitionHash()
	if hash == hex.EncodeToString(legacyHash[:]) {
		t.Fatal("headers did not change the definition hash")
	}
	d.Headers = StaticHeaders{"X-API-Version": "2026-08-14"}
	if d.definitionHash() != hash {
		t.Fatal("header casing changed the definition hash")
	}
	d.Headers["X-API-Version"] = "different"
	if d.definitionHash() == hash {
		t.Fatal("header value did not change the definition hash")
	}
}

func TestStaticHeadersCompilationPreservesDefinition(t *testing.T) {
	for _, name := range []string{"x-api-version", "X-Api-Version"} {
		t.Run(name, func(t *testing.T) {
			d := tokenDefinition()
			d.Headers = StaticHeaders{name: "2026-08-14"}
			expected := StaticHeaders{name: "2026-08-14"}
			d.definitionHash()
			s, err := d.compile()
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Headers) != 1 || s.Headers["X-Api-Version"] != "2026-08-14" {
				t.Fatalf("compiled headers are not canonical: %v", s.Headers)
			}
			s.Headers["X-Api-Version"] = "different"
			if !reflect.DeepEqual(d.Headers, expected) {
				t.Fatalf("hashing or compilation mutated the definition: %v", d.Headers)
			}
		})
	}
}

func TestStaticHeaderUpdatesRequireReconnect(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(s, d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	for _, headers := range []StaticHeaders{{"X-API-Version": "2026-08-14"}, {"X-API-Version": "2026-09-01"}, nil} {
		d.Headers = headers
		if err := s.UpdateCustomService(d); err != nil {
			t.Fatal(err)
		}
		reopened := &Store{Dir: s.Dir, Vault: s.Vault}
		state, err := reopened.RegistrySnapshot()
		if err != nil || len(state.Connections) != 0 {
			t.Fatalf("header update left an active connection: %+v (%v)", state, err)
		}
		stored, err := reopened.read()
		if err != nil || !reflect.DeepEqual(stored.Connections, before.Connections) {
			t.Fatalf("header update lost saved connection: %+v (%v)", stored, err)
		}
		if token, err := s.Vault.Get(before.Connections[0].CredentialRef); err != nil || token != "private-api-key" {
			t.Fatal("header update lost saved credential")
		}
		client := &Client{Store: reopened}
		if _, err := client.Call(context.Background(), d.ID, "GET", "/items"); err == nil || !strings.Contains(err.Error(), "connect") {
			t.Fatalf("call did not require reconnecting: %v", err)
		}
		if err := saveTestService(s, d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
			t.Fatal(err)
		}
		before, err = s.read()
		if err != nil {
			t.Fatal(err)
		}
	}
	d.Headers = StaticHeaders{"X-Test": "ok"}
	if err := s.UpdateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(s, d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	d.Headers = StaticHeaders{"x-test": "ok"}
	d.Endpoints[0].Description = "Updated description"
	if err := s.UpdateCustomService(d); err != nil {
		t.Fatal(err)
	}
	state, err := s.RegistrySnapshot()
	if err != nil || len(state.Connections) != 1 {
		t.Fatal("casing or description update invalidated connection")
	}
	d.BaseURL = "https://other.test"
	if err := s.UpdateCustomService(d); err == nil {
		t.Fatal("header update allowed changing the destination")
	}
}
