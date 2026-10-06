package midwing

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// PasswordAuth describes a JSON login protocol, independently of the backend framework.
// Paths stay on the service's fixed origin. TokenField may be a dotted JSON path.
type PasswordAuth struct {
	LoginPath     string `json:"loginPath"`
	EmailField    string `json:"emailField"`
	PasswordField string `json:"passwordField"`
	TokenField    string `json:"tokenField"`
	RefreshPath   string `json:"refreshPath"` // Optional: POST with the current authorization header; returns TokenField.
}
type Endpoint struct {
	Parameters []string // Allowed string query parameters for custom endpoints.
	Paths      []string // CLI path templates for agent discovery.
	Permission string
	Pattern    *regexp.Regexp
	Queries    map[string]int // Positive integer pagination parameters and their maximum (0 = unbounded).
}
type Permission struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}
type Service struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Origin         string        `json:"origin"`
	AuthKind       string        `json:"authKind"`
	Permissions    []Permission  `json:"permissions"`
	Examples       []string      `json:"examples"`
	Password       *PasswordAuth `json:"-"`
	Header         string        `json:"-"`
	Prefix         string        `json:"-"`
	Endpoints      []Endpoint    `json:"-"`
	Custom         bool          `json:"custom"`
	DefinitionHash string        `json:"-"`
}

var services = []Service{}

func validatePermissions(s Service, permissions []string) error {
	if len(permissions) == 0 {
		return errors.New("select at least one permission")
	}
	for _, p := range permissions {
		if !slices.ContainsFunc(s.Permissions, func(known Permission) bool { return known.ID == p }) {
			return errors.New("unknown permission")
		}
	}
	return nil
}
func (s Service) allowed(method, path string) (string, string, error) {
	if method != "GET" {
		return "", "", errors.New("only GET is supported")
	}
	u, e := url.Parse(path)
	if e != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(path, "\\\r\n") {
		return "", "", errors.New("use a supported relative API path")
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "." || seg == ".." {
			return "", "", errors.New("invalid path")
		}
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return "", "", errors.New("invalid query")
	}
	for _, ep := range s.Endpoints {
		if !ep.Pattern.MatchString(u.Path) {
			continue
		}
		for k, v := range q {
			if slices.Contains(ep.Parameters, k) {
				if len(v) != 1 || len(v[0]) > 2048 {
					return "", "", errors.New("invalid query value")
				}
				continue
			}
			max, ok := ep.Queries[k]
			if !ok || len(v) != 1 {
				return "", "", errors.New("unsupported query parameter")
			}
			n, e := strconv.Atoi(v[0])
			if e != nil || n < 1 || (max > 0 && n > max) {
				return "", "", errors.New("invalid pagination")
			}
		}
		return ep.Permission, u.Path, nil
	}
	return "", "", errors.New("endpoint is not supported")
}
