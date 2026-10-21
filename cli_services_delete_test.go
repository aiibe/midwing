package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

type cleanupFailureVault struct{ cliVault }

func (cleanupFailureVault) Delete(string) error { return errors.New("vault unavailable") }

func TestServicesDelete(t *testing.T) {
	for _, tc := range []struct {
		name           string
		connected      bool
		cleanupFailure bool
	}{
		{name: "disconnected"},
		{name: "connected", connected: true},
		{name: "cleanup pending", connected: true, cleanupFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
			var definition midwing.CustomService
			if err := json.Unmarshal([]byte(serviceJSON), &definition); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"example", "other"} {
				definition.ID = id
				if err := store.CreateCustomService(definition); err != nil {
					t.Fatal(err)
				}
				if tc.connected {
					if err := store.SaveService(id, id+"-token", []string{"endpoint-1"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			vault := store.Vault.(cliVault)
			if tc.cleanupFailure {
				store.Vault = cleanupFailureVault{vault}
			}
			var out bytes.Buffer
			if err := runCLIWithInput([]string{"services", "delete", "example"}, store, nil, &out); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Deleted bool   `json:"deleted"`
				ID      string `json:"id"`
				Warning string `json:"warning"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Deleted || result.ID != "example" || (result.Warning != "") != tc.cleanupFailure {
				t.Fatalf("unexpected deletion result: %s (%v)", out.String(), err)
			}
			state, err := (&midwing.Store{Dir: store.Dir}).RegistrySnapshot()
			if err != nil || len(state.CustomServices) != 1 || state.CustomServices[0].ID != "other" {
				t.Fatalf("deletion did not preserve other service: %+v (%v)", state, err)
			}
			wantConnections, wantCredentials := 0, 0
			if tc.connected {
				wantConnections, wantCredentials = 1, 1
				if tc.cleanupFailure {
					wantCredentials++
				}
			}
			if len(state.Connections) != wantConnections || len(vault) != wantCredentials {
				t.Fatalf("unexpected connections or credentials: %d, %d", len(state.Connections), len(vault))
			}
			if tc.connected {
				conn := state.Connections[0]
				if conn.Service != "other" || vault[conn.CredentialRef] != "other-token" {
					t.Fatal("deletion changed the other service's connection or credential")
				}
			}
			out.Reset()
			if err := runCLIWithInput([]string{"services", "delete", "example"}, store, nil, &out); err == nil || out.Len() != 0 {
				t.Fatal("missing service deletion succeeded or wrote output")
			}
		})
	}
}

func TestServicesDeleteArguments(t *testing.T) {
	for _, args := range [][]string{{"delete"}, {"delete", ""}, {"delete", "--unknown"}, {"delete", "example", "extra"}} {
		var out bytes.Buffer
		if err := runServicesCLI(args, nil, nil, &out); err == nil || !strings.Contains(err.Error(), "usage:") || out.Len() != 0 {
			t.Fatalf("unexpected argument handling for %v: %v, %s", args, err, out.String())
		}
	}
	var out bytes.Buffer
	if err := runServicesCLI([]string{"delete", "--help"}, nil, nil, &out); err != nil || !strings.Contains(out.String(), "midwing services delete ID") {
		t.Fatalf("delete help failed: %v, %s", err, out.String())
	}
}

func TestServicesDeleteExistingCommandName(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir()}
	input := strings.Replace(serviceJSON, `"id":"example"`, `"id":"delete"`, 1)
	var out bytes.Buffer
	if err := runServicesCLI([]string{"create", "-"}, store, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runServicesCLI([]string{"delete", "--config"}, store, nil, &out); err != nil || !strings.Contains(out.String(), `"id":"delete"`) {
		t.Fatalf("existing delete service cannot be inspected: %v, %s", err, out.String())
	}
	out.Reset()
	if err := runServicesCLI([]string{"delete", "delete"}, store, nil, &out); err != nil || !strings.Contains(out.String(), `"deleted":true`) {
		t.Fatalf("existing delete service cannot be deleted: %v, %s", err, out.String())
	}
}
