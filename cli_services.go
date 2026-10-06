package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"midwing/internal/midwing"
	"os"
	"strings"
)

const cliHelp = `midwing                              Open desktop app
midwing services                     List service IDs and connection status
midwing services ID                  Discover endpoints and whether enabled
midwing services ID --config         Show full configuration and permissions
midwing services schema              Show definition schema and examples
midwing services validate FILE|-     Validate without saving
midwing services create FILE|-       Create a custom service
midwing services update FILE|-       Update endpoint descriptions
midwing get SERVICE PATH             Call an enabled GET endpoint
Use --help after a command for details.`

const servicesHelp = `midwing services
midwing services ID [--config]
midwing services schema
midwing services validate FILE|-
midwing services create FILE|-
midwing services update FILE|-
Use - to read one JSON definition from stdin. Configure credentials in the desktop app.`

type serviceSummary struct {
	ID        string `json:"id"`
	Connected bool   `json:"connected"`
}

type endpointView struct {
	Paths           []string       `json:"paths"`
	QueryParameters []string       `json:"queryParameters,omitempty"`
	Pagination      map[string]int `json:"pagination,omitempty"`
	Enabled         bool           `json:"enabled"`
}

type serviceDiscovery struct {
	serviceSummary
	Endpoints []endpointView `json:"endpoints"`
}

type serviceConfig struct {
	midwing.Service
	Definition *midwing.CustomService `json:"definition,omitempty"`
	Connected  bool                   `json:"connected"`
}

func connectionPermissions(connections []midwing.Connection, service midwing.Service) (bool, map[string]bool) {
	permissions := map[string]bool{}
	for _, conn := range connections {
		if conn.Matches(service) {
			for _, permission := range conn.Permissions {
				permissions[permission] = true
			}
			return true, permissions
		}
	}
	return false, permissions
}

func readServiceDefinition(args []string, in io.Reader) (midwing.CustomService, error) {
	var definition midwing.CustomService
	if len(args) != 1 || args[0] == "" || (strings.HasPrefix(args[0], "-") && args[0] != "-") {
		return definition, errors.New("provide one JSON file path or - for stdin")
	}
	if args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return definition, fmt.Errorf("could not open service definition: %w", err)
		}
		defer file.Close()
		in = file
	}
	data, err := io.ReadAll(io.LimitReader(in, 1024*1024+1))
	if err != nil {
		return definition, err
	}
	if len(data) > 1024*1024 {
		return definition, errors.New("service definition exceeds 1 MiB")
	}
	// Decode into a pointer to reject JSON null as well as unknown fields.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value *midwing.CustomService
	if err = decoder.Decode(&value); err != nil {
		return definition, fmt.Errorf("invalid service JSON: %w", err)
	}
	if value == nil {
		return definition, errors.New("service definition must be a JSON object")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return definition, errors.New("service definition must contain exactly one JSON object")
	}
	if err := validateDefinitionJSON(data); err != nil {
		return definition, fmt.Errorf("invalid service JSON: %w", err)
	}
	return *value, nil
}

func runServicesCLI(args []string, store *midwing.Store, in io.Reader, out io.Writer) error {
	encode := func(v any) error { return json.NewEncoder(out).Encode(v) }
	if len(args) > 0 && args[len(args)-1] == "--help" {
		if len(args) == 1 {
			_, err := fmt.Fprintln(out, servicesHelp)
			return err
		}
		if len(args) == 2 && (args[0] == "create" || args[0] == "validate" || args[0] == "update") {
			_, err := fmt.Fprintf(out, "midwing services %s FILE|-\nRead one JSON definition (max 1 MiB). Use services schema for fields and examples. Validation writes no settings; creation rejects existing IDs; update only changes endpoint descriptions on an existing custom service. Credentials are entered in the desktop app.\n", args[0])
			return err
		}
		if len(args) == 2 && args[0] == "schema" {
			_, err := fmt.Fprintln(out, "midwing services schema\nReturns a JSON Schema, input defaults, constraints, and token/password examples.")
			return err
		}
		if len(args) == 2 {
			_, err := fmt.Fprintln(out, "midwing services ID [--config]\nShow endpoints with enabled status. --config includes the full definition and permissions.")
			return err
		}
		return errors.New("invalid help command; run midwing services --help")
	}
	if len(args) == 1 && args[0] == "schema" {
		return encode(serviceSchema())
	}
	if len(args) > 0 && (args[0] == "create" || args[0] == "validate" || args[0] == "update") {
		definition, err := readServiceDefinition(args[1:], in)
		if err != nil {
			return err
		}
		if args[0] == "validate" {
			if _, err = midwing.ValidateCustomService(definition); err != nil {
				return fmt.Errorf("invalid service definition: %w; run midwing services schema for fields and examples", err)
			}
			return encode(struct {
				Valid bool   `json:"valid"`
				ID    string `json:"id"`
			}{true, definition.ID})
		}
		if args[0] == "update" {
			if err = store.UpdateCustomService(definition); err != nil {
				return err
			}
			return encode(struct {
				Updated bool   `json:"updated"`
				ID      string `json:"id"`
			}{true, definition.ID})
		}
		if err = store.CreateCustomService(definition); err != nil {
			var invalid *midwing.InvalidServiceDefinitionError
			if errors.As(err, &invalid) {
				return fmt.Errorf("invalid service definition: %w; run midwing services schema for fields and examples", err)
			}
			return err
		}
		return encode(struct {
			serviceSummary
			Next string `json:"next"`
		}{serviceSummary{ID: definition.ID}, "Connect this service and select permissions in the desktop app."})
	}
	if len(args) > 2 || (len(args) == 2 && args[1] != "--config") {
		return errors.New("use midwing services ID [--config]; run midwing services --help")
	}
	state, err := store.RegistrySnapshot()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		rows := []serviceSummary{}
		for _, service := range state.Services {
			connected, _ := connectionPermissions(state.Connections, service)
			rows = append(rows, serviceSummary{service.ID, connected})
		}
		return encode(rows)
	}
	for _, service := range state.Services {
		if service.ID != args[0] {
			continue
		}
		connected, permissions := connectionPermissions(state.Connections, service)
		if len(args) == 2 {
			var definition *midwing.CustomService
			for i := range state.CustomServices {
				if state.CustomServices[i].ID == service.ID {
					definition = &state.CustomServices[i]
					break
				}
			}
			return encode(serviceConfig{service, definition, connected})
		}
		view := serviceDiscovery{serviceSummary{service.ID, connected}, []endpointView{}}
		for _, endpoint := range service.Endpoints {
			view.Endpoints = append(view.Endpoints, endpointView{endpoint.Paths, endpoint.Parameters, endpoint.Queries, permissions[endpoint.Permission]})
		}
		return encode(view)
	}
	return fmt.Errorf("unknown service %q; run midwing services to list configured IDs", args[0])
}
