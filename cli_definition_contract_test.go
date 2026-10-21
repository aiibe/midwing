package main

import (
	"bytes"
	"strings"
	"testing"

	"midwing/internal/midwing"
)

func TestDefinitionJSONContract(t *testing.T) {
	for _, input := range []string{
		strings.Replace(serviceJSON, `"prefix":"Bearer "`, `"prefix":null`, 1),
		strings.Replace(serviceJSON, `"prefix":"Bearer "`, `"prefix":"Bearer ","password":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"queryParameters":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"queryParameters":[null]`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":null`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":[null]`, 1),
		strings.Replace(serviceJSON, `"queryParameters":["limit"]`, `"methods":[]`, 1),
		strings.Replace(serviceJSON, `"id":`, `"ID":`, 1),
		strings.Replace(serviceJSON, `"path":"/items/{id}"`, `"path":null`, 1),
		strings.Replace(serviceJSON, `"authKind":"token"`, `"authKind":"password","password":{"loginPath":"/login","emailField":"email","passwordField":"password","tokenField":"token","refreshPath":null}`, 1),
	} {
		for _, command := range []string{"create", "validate"} {
			var out bytes.Buffer
			if err := runCLIWithInput([]string{"services", command, "-"}, &midwing.Store{Dir: t.TempDir()}, strings.NewReader(input), &out); err == nil || out.Len() != 0 {
				t.Fatalf("invalid contract accepted: %s", input)
			}
		}
	}
	omitted := strings.Replace(serviceJSON, `,"prefix":"Bearer "`, "", 1)
	omitted = strings.Replace(omitted, `,"queryParameters":["limit"]`, "", 1)
	var out bytes.Buffer
	if err := runCLIWithInput([]string{"services", "validate", "-"}, nil, strings.NewReader(omitted), &out); err != nil {
		t.Fatal("omission defaults rejected:", err)
	}
}

func TestAPICommandsRejectMalformedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"get"},
		{"get", "example"},
		{"get", "example", "/items", "{}"},
		{"post"},
		{"post", "example"},
		{"post", "example", "/items"},
		{"post", "example", "/items", "{}", "{}"},
	} {
		var out bytes.Buffer
		err := runCLIWithInput(args, nil, nil, &out)
		if err == nil || !strings.Contains(err.Error(), "usage: midwing "+args[0]) || out.Len() != 0 {
			t.Fatalf("malformed command %q: error = %v, output = %q", args, err, out.String())
		}
	}
}
