package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Preserve omission defaults while rejecting explicit null and undocumented keys.
func definitionObject(data []byte, required, optional string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("definition fields must be JSON objects")
	}
	allowed := map[string]bool{}
	for _, key := range strings.Fields(required + " " + optional) {
		allowed[key] = true
	}
	for key, value := range fields {
		if !allowed[key] {
			return nil, fmt.Errorf("unknown field %q", key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("field %q must not be null", key)
		}
	}
	for _, key := range strings.Fields(required) {
		if _, ok := fields[key]; !ok {
			return nil, fmt.Errorf("missing required field %q", key)
		}
	}
	return fields, nil
}

func validateDefinitionJSON(data []byte) error {
	fields, err := definitionObject(data, "id name baseUrl authKind header endpoints", "prefix password")
	if err != nil {
		return err
	}
	var kind string
	if err := json.Unmarshal(fields["authKind"], &kind); err != nil {
		return err
	}
	password, present := fields["password"]
	if kind == "token" && present {
		return fmt.Errorf("token services must not include password")
	}
	if kind == "password" && !present {
		return fmt.Errorf("password services require password")
	}
	if present {
		if _, err := definitionObject(password, "loginPath emailField passwordField tokenField", "refreshPath"); err != nil {
			return err
		}
	}
	var endpoints []json.RawMessage
	if err := json.Unmarshal(fields["endpoints"], &endpoints); err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		if _, err := definitionObject(endpoint, "path", "description queryParameters"); err != nil {
			return err
		}
	}
	return nil
}
