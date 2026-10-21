package midwing

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var literalSegment = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)

func compilePath(path string, templates bool) (*regexp.Regexp, string, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || len(path) > 1024 {
		return nil, "", errors.New("paths must start with a single / and contain at most 1024 characters")
	}
	segments := strings.Split(path[1:], "/")
	pattern := "^"
	hint := ""
	seen := map[string]bool{}
	for _, segment := range segments {
		pattern += "/"
		hint += "/"
		if templates && strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			if !fieldName.MatchString(name) || seen[name] {
				return nil, "", errors.New("use unique path placeholders such as {id}")
			}
			seen[name] = true
			pattern += "[A-Za-z0-9_~-][A-Za-z0-9_.~-]*"
			hint += strings.ToUpper(name)
			continue
		}
		if segment == "" && path != "/" || segment == "." || segment == ".." || segment != "" && !literalSegment.MatchString(segment) {
			return nil, "", errors.New("paths must contain plain segments or {name} placeholders, without queries or traversal")
		}
		pattern += regexp.QuoteMeta(segment)
		hint += segment
	}
	return regexp.MustCompile(pattern + "$"), hint, nil
}

func (d CustomService) compileEndpoints() (Service, error) {
	if len(d.Endpoints) == 0 || len(d.Endpoints) > 100 {
		return Service{}, errors.New("add between 1 and 100 endpoints")
	}
	s := Service{
		ID: d.ID, Name: strings.TrimSpace(d.Name), Description: "Custom API service",
		Origin: strings.TrimSuffix(d.BaseURL, "/"), AuthKind: d.AuthKind,
		Header: http.CanonicalHeaderKey(d.Header), Prefix: d.Prefix,
		Headers: d.Headers.canonical(), Password: d.Password, Custom: true,
	}
	seen := map[string]bool{}
	for i, endpoint := range d.Endpoints {
		pattern, hint, e := compilePath(endpoint.Path, true)
		if e != nil {
			return Service{}, fmt.Errorf("endpoint %d: %w", i+1, e)
		}
		if seen[pattern.String()] {
			return Service{}, errors.New("duplicate endpoint")
		}
		for _, previous := range d.Endpoints[:i] {
			if pathsOverlap(previous.Path, endpoint.Path) {
				return Service{}, errors.New("overlapping endpoints are not supported")
			}
		}
		seen[pattern.String()] = true
		if d.Password != nil && (pattern.MatchString(d.Password.LoginPath) || d.Password.RefreshPath != "" && pattern.MatchString(d.Password.RefreshPath)) {
			return Service{}, errors.New("login and refresh paths cannot be exposed as API endpoints")
		}
		if len(endpoint.QueryParameters) > 30 {
			return Service{}, errors.New("too many query parameters")
		}
		keys := map[string]bool{}
		for _, key := range endpoint.QueryParameters {
			if !fieldName.MatchString(key) || keys[key] {
				return Service{}, errors.New("query parameters must have unique field names")
			}
			keys[key] = true
		}
		requests, permissions, err := endpoint.compilePermissions(i + 1)
		if err != nil {
			return Service{}, fmt.Errorf("endpoint %d: %w", i+1, err)
		}
		s.Permissions = append(s.Permissions, permissions...)
		s.Endpoints = append(s.Endpoints, Endpoint{Paths: []string{hint}, MethodPermissions: requests, Pattern: pattern, Parameters: endpoint.QueryParameters})
		s.Examples = append(s.Examples, hint)
	}
	s.DefinitionHash = d.definitionHash()
	return s, nil
}

// sortedMethods applies the omission default without changing the definition.
func (e CustomEndpoint) sortedMethods() []string {
	methods := slices.Clone(e.Methods)
	if methods == nil {
		methods = []string{"GET"}
	}
	slices.Sort(methods)
	return methods
}

func (e CustomEndpoint) compilePermissions(index int) (map[string]string, []Permission, error) {
	methods := e.sortedMethods()
	if len(methods) == 0 || len(methods) > 2 {
		return nil, nil, errors.New("methods may contain GET and POST only")
	}
	for i, method := range methods {
		if method != "GET" && method != "POST" || i > 0 && method == methods[i-1] {
			return nil, nil, errors.New("methods must contain unique GET and/or POST values")
		}
	}
	description := strings.TrimSpace(e.Description)
	if utf8.RuneCountInString(description) > 500 {
		return nil, nil, errors.New("description must contain at most 500 characters")
	}
	requests := make(map[string]string, len(methods))
	permissions := make([]Permission, 0, len(methods))
	for _, method := range methods {
		id := fmt.Sprintf("endpoint-%d", index)
		defaultDescription := "Read " + e.Path
		if method == "POST" {
			id += "-post"
			defaultDescription = "Submit a request to " + e.Path
		}
		methodDescription := description
		if methodDescription == "" {
			methodDescription = defaultDescription
		}
		requests[method] = id
		permissions = append(permissions, Permission{ID: id, Label: method + " " + e.Path, Description: methodDescription})
	}
	return requests, permissions, nil
}

// Paths have already been validated: each segment is a literal or one placeholder.
func pathsOverlap(a, b string) bool {
	left, right := strings.Split(a, "/"), strings.Split(b, "/")
	if len(left) != len(right) {
		return false
	}
	for i, x := range left {
		y := right[i]
		if x == y {
			continue
		}
		xt, yt := strings.HasPrefix(x, "{"), strings.HasPrefix(y, "{")
		if xt && yt {
			continue
		}
		// Placeholder values cannot begin with a dot, although literal segments can.
		if xt && !strings.HasPrefix(y, ".") || yt && !strings.HasPrefix(x, ".") {
			continue
		}
		return false
	}
	return true
}
