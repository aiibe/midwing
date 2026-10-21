package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

func TestServicesStaticHeaders(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	input := strings.Replace(serviceJSON, `"endpoints":`, `"headers":{"X-API-Version":"2026-08-14"},"endpoints":`, 1)
	var out bytes.Buffer
	for _, command := range []string{"validate", "create"} {
		out.Reset()
		if err := runServicesCLI([]string{command, "-"}, store, strings.NewReader(input), &out); err != nil {
			t.Fatalf("%s rejected static headers: %v", command, err)
		}
	}
	if err := store.SaveService("example", "private-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	for _, definition := range []string{
		strings.Replace(input, "2026-08-14", "2026-09-01", 1),
		strings.Replace(input, `{"X-API-Version":"2026-08-14"}`, `{}`, 1),
		input,
		serviceJSON,
	} {
		out.Reset()
		if err := runServicesCLI([]string{"update", "-"}, store, strings.NewReader(definition), &out); err != nil {
			t.Fatalf("header update failed: %v", err)
		}
		out.Reset()
		if err := runServicesCLI([]string{"example", "--config"}, store, nil, &out); err != nil {
			t.Fatal(err)
		}
		var config serviceConfig
		if err := json.Unmarshal(out.Bytes(), &config); err != nil || config.Connected || config.Definition == nil {
			t.Fatalf("updated service should require reconnection: %s (%v)", out.String(), err)
		}
		var expected midwing.CustomService
		if err := json.Unmarshal([]byte(definition), &expected); err != nil {
			t.Fatal(err)
		}
		if config.Definition.Headers["X-API-Version"] != expected.Headers["X-API-Version"] {
			t.Fatalf("header update was not persisted: %s", out.String())
		}
		if err := store.SaveService("example", "private-token", []string{"endpoint-1"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServicesRejectInvalidStaticHeaders(t *testing.T) {
	for _, headers := range []string{
		`null`, `[]`, `"text"`, `{"X-Test":null}`, `{"X-Test":123}`, `{"X-Test":false}`,
		`{"Bad Name":"value"}`, `{"Bad\rName":"value"}`, `{"Bad\nName":"value"}`,
		`{"X-Test":"bad\rvalue"}`, `{"X-Test":"bad\nvalue"}`,
		`{"authorization":"override"}`, `{"Host":"other.test"}`, `{"Content-Type":"text/plain"}`,
		`{"X-Test":"one","x-test":"two"}`, `{"X-Test":"one","X-Test":"two"}`,
	} {
		t.Run(headers, func(t *testing.T) {
			input := strings.Replace(serviceJSON, `"endpoints":`, `"headers":`+headers+`,"endpoints":`, 1)
			for _, command := range []string{"validate", "create", "update"} {
				t.Run(command, func(t *testing.T) {
					store := &midwing.Store{Dir: t.TempDir()}
					expectedServices := 0
					if command == "update" {
						if err := runServicesCLI([]string{"create", "-"}, store, strings.NewReader(serviceJSON), &bytes.Buffer{}); err != nil {
							t.Fatal(err)
						}
						expectedServices = 1
					}
					var out bytes.Buffer
					if err := runServicesCLI([]string{command, "-"}, store, strings.NewReader(input), &out); err == nil || out.Len() != 0 {
						t.Fatalf("accepted invalid headers or wrote output: %v, %s", err, out.String())
					}
					state, err := store.RegistrySnapshot()
					if err != nil || len(state.CustomServices) != expectedServices || expectedServices > 0 && len(state.CustomServices[0].Headers) != 0 {
						t.Fatalf("invalid headers changed settings: %+v (%v)", state, err)
					}
				})
			}
		})
	}
}

func TestServicesUpdateEndpointsAndHeaders(t *testing.T) {
	store := &midwing.Store{Dir: t.TempDir(), Vault: cliVault{}}
	var out bytes.Buffer
	if err := runServicesCLI([]string{"create", "-"}, store, strings.NewReader(serviceJSON), &out); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveService("example", "private-token", []string{"endpoint-1"}); err != nil {
		t.Fatal(err)
	}
	var expanded midwing.CustomService
	if err := json.Unmarshal([]byte(serviceJSON), &expanded); err != nil {
		t.Fatal(err)
	}
	expanded.Headers = midwing.StaticHeaders{"X-API-Version": "2026-08-14"}
	expanded.Endpoints = []midwing.CustomEndpoint{
		{Path: "/models", QueryParameters: []string{}},
		{Path: "/tts", Methods: []string{"POST"}, QueryParameters: []string{"language"}},
	}
	input, err := json.Marshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"validate", "update"} {
		out.Reset()
		if err := runServicesCLI([]string{command, "-"}, store, bytes.NewReader(input), &out); err != nil {
			t.Fatalf("%s rejected expanded definition: %v", command, err)
		}
	}
	out.Reset()
	if err := runServicesCLI([]string{"example", "--config"}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var config serviceConfig
	if err := json.Unmarshal(out.Bytes(), &config); err != nil || config.Connected || config.Definition == nil {
		t.Fatalf("invalid updated configuration: %s (%v)", out.String(), err)
	}
	actual, err := json.Marshal(config.Definition)
	if err != nil || !bytes.Equal(actual, input) {
		t.Fatalf("expanded definition was not saved: %s (%v)", actual, err)
	}
	if err := store.SaveService("example", "private-token", []string{"endpoint-1", "endpoint-2-post"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runServicesCLI([]string{"example"}, store, nil, &out); err != nil {
		t.Fatal(err)
	}
	var discovery serviceDiscovery
	if err := json.Unmarshal(out.Bytes(), &discovery); err != nil || !discovery.Connected || len(discovery.Endpoints) != 2 {
		t.Fatalf("invalid reconnected discovery: %s (%v)", out.String(), err)
	}
	if !discovery.Endpoints[0].Enabled || !discovery.Endpoints[1].Enabled || discovery.Endpoints[1].Requests[0].Method != "POST" {
		t.Fatalf("expanded permissions were not enabled: %s", out.String())
	}
}
