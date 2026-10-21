package midwing

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func approvalDefinition(password bool) CustomService {
	d := tokenDefinition()
	d.Endpoints = []CustomEndpoint{{Path: "/public"}, {Path: "/other"}}
	if password {
		d.AuthKind = "password"
		d.Password = &PasswordAuth{LoginPath: "/login", EmailField: "email", PasswordField: "password", TokenField: "token"}
	}
	return d
}

func TestStaleConnectionApprovals(t *testing.T) {
	changes := []struct {
		name   string
		change func(*CustomService)
	}{
		{"reused permission ID", func(d *CustomService) { d.Endpoints[0].Path = "/admin/export" }},
		{"methods", func(d *CustomService) { d.Endpoints[0].Methods = []string{"GET", "POST"} }},
		{"query parameters", func(d *CustomService) { d.Endpoints[0].QueryParameters = []string{"admin"} }},
		{"headers", func(d *CustomService) { d.Headers = StaticHeaders{"X-Role": "admin"} }},
		{"reorder", func(d *CustomService) { d.Endpoints[0], d.Endpoints[1] = d.Endpoints[1], d.Endpoints[0] }},
		{"missing revision", func(*CustomService) {}},
	}
	for _, password := range []bool{false, true} {
		kind := "token"
		if password {
			kind = "password"
		}
		for _, tc := range changes {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				s := testStore(t)
				d := approvalDefinition(password)
				if err := s.CreateCustomService(d); err != nil {
					t.Fatal(err)
				}
				reviewed, err := s.Service(d.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.saveToken(d.ID, "existing-token", []string{"endpoint-1"}, &reviewed); err != nil {
					t.Fatal(err)
				}
				// The app still displays /public while another process updates its permission ID.
				tc.change(&d)
				if err := s.UpdateCustomService(d); err != nil {
					t.Fatal(err)
				}
				before, err := s.read()
				if err != nil {
					t.Fatal(err)
				}
				vaultBefore := map[string]string{}
				for k, v := range s.Vault.(memoryVault) {
					vaultBefore[k] = v
				}
				revision := reviewed.DefinitionHash
				if tc.name == "missing revision" {
					revision = ""
				}
				client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
					t.Fatal("stale approval sent credentials to the network")
					return nil, nil
				})}}
				if password {
					err = client.ConnectPasswordRevision(context.Background(), d.ID, revision, "me@example.com", "password-secret", []string{"endpoint-1"})
				} else {
					err = s.SaveServiceRevision(d.ID, revision, "new-token", []string{"endpoint-1"})
				}
				if err == nil || !strings.Contains(err.Error(), "review permissions") {
					t.Fatalf("stale approval accepted: %v", err)
				}
				after, err := s.read()
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("rejected approval changed settings: %v", err)
				}
				if !reflect.DeepEqual(vaultBefore, map[string]string(s.Vault.(memoryVault))) {
					t.Fatal("rejected approval changed credentials")
				}
			})
		}
	}
}

func TestReviewedConnectionApprovals(t *testing.T) {
	for _, password := range []bool{false, true} {
		for _, descriptionOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("password=%t/description=%t", password, descriptionOnly), func(t *testing.T) {
				s := testStore(t)
				d := approvalDefinition(password)
				if err := s.CreateCustomService(d); err != nil {
					t.Fatal(err)
				}
				if !descriptionOnly {
					d.Endpoints[0].Path = "/admin/export"
					if err := s.UpdateCustomService(d); err != nil {
						t.Fatal(err)
					}
				}
				state, err := s.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				var revision string
				for _, service := range state.Services {
					if service.ID == d.ID {
						revision = service.Revision
					}
				}
				if descriptionOnly {
					if err := s.UpdatePermissionDescriptions(d.ID, map[string]string{"/public": "New description"}); err != nil {
						t.Fatal(err)
					}
				}
				client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != "/v1/login" {
						t.Fatalf("unexpected login: %s", r.URL.Path)
					}
					return &http.Response{StatusCode: 200, Body: ioBody(`{"token":"new-token"}`)}, nil
				})}}
				if password {
					err = client.ConnectPasswordRevision(context.Background(), d.ID, revision, "me@example.com", "password-secret", []string{"endpoint-1"})
				} else {
					err = s.SaveServiceRevision(d.ID, revision, "new-token", []string{"endpoint-1"})
				}
				if err != nil {
					t.Fatal(err)
				}
				connections, err := s.List()
				if err != nil || len(connections) != 1 || connections[0].DefinitionHash != revision {
					t.Fatalf("reviewed approval failed: %+v (%v)", connections, err)
				}
			})
		}
	}
}

func TestPasswordApprovalChangesDuringLogin(t *testing.T) {
	s := testStore(t)
	d := approvalDefinition(true)
	if err := s.CreateCustomService(d); err != nil {
		t.Fatal(err)
	}
	reviewed, err := s.Service(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		d.Endpoints[0].Path = "/admin/export"
		if err := s.UpdateCustomService(d); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Body: ioBody(`{"token":"new-token"}`)}, nil
	})}}
	err = client.ConnectPasswordRevision(context.Background(), d.ID, reviewed.DefinitionHash, "me@example.com", "password-secret", []string{"endpoint-1"})
	if err == nil || !strings.Contains(err.Error(), "review permissions") {
		t.Fatalf("login saved stale approval: %v", err)
	}
	cfg, err := s.read()
	if err != nil || len(cfg.Connections) != 0 || len(s.Vault.(memoryVault)) != 0 {
		t.Fatal("stale login persisted credentials or connection")
	}
}
