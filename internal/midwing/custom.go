package midwing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type CustomEndpoint struct {
	Description     string   `json:"description,omitempty"`
	Path            string   `json:"path"`
	QueryParameters []string `json:"queryParameters"`
}
type CustomService struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	BaseURL   string           `json:"baseUrl"`
	AuthKind  string           `json:"authKind"`
	Header    string           `json:"header"`
	Prefix    string           `json:"prefix"`
	Password  *PasswordAuth    `json:"password,omitempty"`
	Endpoints []CustomEndpoint `json:"endpoints"`
}

// ValidateCustomService uses the same definition validator as desktop creation.
func ValidateCustomService(d CustomService) (Service, error) { return d.compile() }

var customID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var fieldName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var jsonPath = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
var credentialPrefix = regexp.MustCompile(`^[ -~]*$`)
var literalSegment = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)

func validBaseURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(raw, "\\\r\n #") {
		return false
	}
	if u.Path != "" && u.Path != "/" {
		_, _, e = compilePath(strings.TrimSuffix(u.Path, "/"), false)
		if e != nil {
			return false
		}
	}
	return true
}
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
func (d CustomService) compile() (Service, error) {
	if err := d.validateMetadata(); err != nil {
		return Service{}, err
	}
	if err := d.validateAuth(); err != nil {
		return Service{}, err
	}
	return d.compileEndpoints()
}

func (d CustomService) validateMetadata() error {
	if d.ID == "schema" || d.ID == "create" || d.ID == "validate" || d.ID == "update" {
		return errors.New("service ID is reserved by the CLI; choose an ID other than schema, create, validate, or update")
	}
	if !customID.MatchString(d.ID) {
		return errors.New("service ID must start with a letter and use lowercase letters, numbers, or hyphens (max 48)")
	}
	if len(strings.TrimSpace(d.Name)) == 0 || len(d.Name) > 80 || strings.ContainsAny(d.Name, "\r\n") {
		return errors.New("enter a service name (max 80 characters)")
	}
	if !validBaseURL(d.BaseURL) {
		return errors.New("base URL must be HTTPS without credentials, queries, or fragments")
	}
	return nil
}

func (d CustomService) validateAuth() error {
	if !headerName.MatchString(d.Header) || len(d.Header) > 80 {
		return errors.New("enter a valid credential header name")
	}
	switch strings.ToLower(d.Header) {
	case "host", "content-length", "connection", "transfer-encoding", "cookie", "content-type", "accept", "user-agent":
		return errors.New("use Authorization or a dedicated API credential header")
	}
	if len(d.Prefix) > 80 || !credentialPrefix.MatchString(d.Prefix) {
		return errors.New("invalid credential prefix")
	}
	if d.AuthKind != "token" && d.AuthKind != "password" {
		return errors.New("choose API token or email/password")
	}
	if d.AuthKind == "token" && d.Password != nil {
		return errors.New("token services must not include a login contract")
	}
	if d.AuthKind == "password" {
		if d.Password == nil {
			return errors.New("configure the login endpoint and fields")
		}
		p := d.Password
		if _, _, e := compilePath(p.LoginPath, false); e != nil {
			return fmt.Errorf("invalid login path: %w", e)
		}
		if p.RefreshPath != "" {
			if _, _, e := compilePath(p.RefreshPath, false); e != nil {
				return fmt.Errorf("invalid refresh path: %w", e)
			}
		}
		if !fieldName.MatchString(p.EmailField) || !fieldName.MatchString(p.PasswordField) || p.EmailField == p.PasswordField || !jsonPath.MatchString(p.TokenField) {
			return errors.New("use distinct login field names and a token response path such as access_token or session.token")
		}
	}
	return nil
}

func (d CustomService) compileEndpoints() (Service, error) {
	if len(d.Endpoints) == 0 || len(d.Endpoints) > 100 {
		return Service{}, errors.New("add between 1 and 100 GET endpoints")
	}
	s := Service{ID: d.ID, Name: strings.TrimSpace(d.Name), Description: "Custom service · Read-only access", Origin: strings.TrimSuffix(d.BaseURL, "/"), AuthKind: d.AuthKind, Header: http.CanonicalHeaderKey(d.Header), Prefix: d.Prefix, Password: d.Password, Custom: true}
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
			return Service{}, errors.New("login and refresh paths cannot be exposed as read endpoints")
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
		description := strings.TrimSpace(endpoint.Description)
		if utf8.RuneCountInString(description) > 500 {
			return Service{}, fmt.Errorf("endpoint %d: description must contain at most 500 characters", i+1)
		}
		if description == "" {
			description = "Read " + endpoint.Path
		}
		permission := fmt.Sprintf("endpoint-%d", i+1)
		s.Permissions = append(s.Permissions, Permission{ID: permission, Label: "GET " + endpoint.Path, Description: description})
		s.Endpoints = append(s.Endpoints, Endpoint{Paths: []string{hint}, Permission: permission, Pattern: pattern, Parameters: endpoint.QueryParameters})
		s.Examples = append(s.Examples, hint)
	}
	s.DefinitionHash = d.definitionHash()
	return s, nil
}

func (d CustomService) definitionHash() string {
	d.Endpoints = append([]CustomEndpoint(nil), d.Endpoints...)
	for i := range d.Endpoints {
		d.Endpoints[i].Description = ""
	}
	data, _ := json.Marshal(d)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func definitions(cfg Settings) ([]Service, error) {
	out := append([]Service{}, services...)
	seen := map[string]bool{}
	for _, s := range out {
		seen[s.ID] = true
	}
	for _, d := range cfg.CustomServices {
		if seen[d.ID] {
			return nil, errors.New("duplicate or reserved custom service ID in settings")
		}
		s, e := d.compile()
		if e != nil {
			return nil, e
		}
		out = append(out, s)
		seen[d.ID] = true
	}
	return out, nil
}
func resolve(cfg Settings, id string) (Service, error) {
	all, e := definitions(cfg)
	if e != nil {
		return Service{}, e
	}
	return findService(all, id)
}

func findService(all []Service, id string) (Service, error) {
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Service{}, errors.New("unsupported service")
}
func (s *Store) Services() ([]Service, error) {
	var out []Service
	e := s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		out, e = definitions(cfg)
		return e
	})
	return out, e
}
func (s *Store) Service(id string) (Service, error) {
	var out Service
	e := s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		out, e = resolve(cfg, id)
		return e
	})
	return out, e
}

// InvalidServiceDefinitionError distinguishes validation from persistence errors.
type InvalidServiceDefinitionError struct{ Err error }

func (e *InvalidServiceDefinitionError) Error() string { return e.Err.Error() }
func (e *InvalidServiceDefinitionError) Unwrap() error { return e.Err }

func (s *Store) CreateCustomService(d CustomService) error {
	if _, e := d.compile(); e != nil {
		return &InvalidServiceDefinitionError{Err: e}
	}
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		all, e := definitions(cfg)
		if e != nil {
			return e
		}
		for _, known := range all {
			if known.ID == d.ID {
				return errors.New("service ID already exists; choose another ID")
			}
		}
		cfg.CustomServices = append(cfg.CustomServices, d)
		return s.write(cfg)
	})
}
func (s *Store) DeleteCustomService(id string) error {
	return s.locked(func() error {
		cfg, e := s.read()
		if e != nil {
			return e
		}
		index := -1
		for i, d := range cfg.CustomServices {
			if d.ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return errors.New("custom service not found")
		}
		removeConnections(&cfg, id)
		cfg.CustomServices = append(cfg.CustomServices[:index], cfg.CustomServices[index+1:]...)
		if e = s.write(cfg); e != nil {
			return e
		}
		return s.finishCredentialCleanup(cfg)
	})
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

// UpdateCustomService accepts a full definition, but only permits description
// changes so existing credentials and permission grants remain valid.
func (s *Store) UpdateCustomService(d CustomService) error {
	if _, err := d.compile(); err != nil {
		return &InvalidServiceDefinitionError{Err: err}
	}
	return s.locked(func() error {
		cfg, err := s.read()
		if err != nil {
			return err
		}
		for i := range cfg.CustomServices {
			if cfg.CustomServices[i].ID != d.ID {
				continue
			}
			if cfg.CustomServices[i].definitionHash() != d.definitionHash() {
				return errors.New("update only supports endpoint description changes; keep all other definition fields unchanged")
			}
			cfg.CustomServices[i] = d
			return s.write(cfg)
		}
		return errors.New("custom service not found")
	})
}

// UpdatePermissionDescriptions only changes display metadata. Paths identify the
// endpoints so a stale editor cannot overwrite another endpoint's description.
func (s *Store) UpdatePermissionDescriptions(id string, descriptions map[string]string) error {
	return s.locked(func() error {
		cfg, err := s.read()
		if err != nil {
			return err
		}
		for i := range cfg.CustomServices {
			d := &cfg.CustomServices[i]
			if d.ID != id {
				continue
			}
			remaining := len(descriptions)
			for j := range d.Endpoints {
				if description, ok := descriptions[d.Endpoints[j].Path]; ok {
					d.Endpoints[j].Description = strings.TrimSpace(description)
					remaining--
				}
			}
			if remaining != 0 {
				return errors.New("endpoints changed; reload the service and try again")
			}
			if _, err := d.compile(); err != nil {
				return err
			}
			return s.write(cfg)
		}
		return errors.New("custom service not found")
	})
}
