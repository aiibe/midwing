package midwing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPermissionDescriptionsPreserveConnection(t *testing.T) {
	s := testStore(t)
	d := tokenDefinition()
	// The original JSON shape must retain its hash when descriptions are absent.
	legacy, _ := json.Marshal(d)
	if strings.Contains(string(legacy), "description") {
		t.Fatal("empty description changed legacy JSON")
	}
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := saveTestService(s, d.ID, "private-api-key", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	before, err := s.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	d.Endpoints[0].Description = "Updated through CLI"
	d.Endpoints[0].Methods = []string{"GET"}
	if err := s.UpdateCustomService(d); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePermissionDescriptions(d.ID, map[string]string{"/items": "  Read your items  ", "/items/{id}": " "}); err != nil {
		t.Fatal(err)
	}
	reopened := &Store{Dir: s.Dir, Vault: s.Vault}
	after, err := reopened.RegistrySnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Connections) != 1 || after.Connections[0].CredentialRef != before.Connections[0].CredentialRef || after.Connections[0].DefinitionHash != before.Connections[0].DefinitionHash || len(after.Connections[0].Permissions) != 1 {
		t.Fatal("description update changed connection")
	}
	services, err := reopened.Services()
	if err != nil {
		t.Fatal(err)
	}
	custom := services[len(services)-1]
	if custom.Permissions[0].Description != "Read your items" || custom.Permissions[1].Description != "Read /items/{id}" {
		t.Fatalf("unexpected descriptions: %v", custom.Permissions)
	}
	for _, descriptions := range []map[string]string{{"/unknown": "Unknown"}, {"/items": strings.Repeat("x", 501)}} {
		if err := s.UpdatePermissionDescriptions(d.ID, descriptions); err == nil {
			t.Fatal("accepted invalid description update")
		}
	}
	d.Endpoints[0].Description = "Different wording"
	if d.definitionHash() != tokenDefinition().definitionHash() {
		t.Fatal("description changes hash")
	}
	d.Endpoints[0].Description = strings.Repeat("x", 501)
	if _, err := d.compile(); err == nil {
		t.Fatal("accepted excessive description on creation")
	}
}

func TestDefinitionHashTreatsMethodsAsSet(t *testing.T) {
	d := tokenDefinition()
	legacyHash := d.definitionHash()
	d.Endpoints[0].Methods = []string{"GET"}
	if d.definitionHash() != legacyHash {
		t.Fatal("explicit GET changed the legacy hash")
	}
	d.Endpoints[0].Methods = []string{"POST", "GET"}
	writeHash := d.definitionHash()
	if writeHash == legacyHash {
		t.Fatal("adding POST did not change the definition hash")
	}
	if d.Endpoints[0].Methods[0] != "POST" {
		t.Fatal("hashing mutated the definition")
	}
	d.Endpoints[0].Methods = []string{"GET", "POST"}
	if d.definitionHash() != writeHash {
		t.Fatal("method order changed the definition hash")
	}
	d.Endpoints[0].Methods = []string{"POST"}
	if d.definitionHash() == writeHash || d.definitionHash() == legacyHash {
		t.Fatal("POST-only definition did not have a distinct hash")
	}
}
