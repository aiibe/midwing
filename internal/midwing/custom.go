package midwing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

type CustomEndpoint struct {
	Description     string   `json:"description,omitempty"`
	Path            string   `json:"path"`
	Methods         []string `json:"methods,omitempty"`
	QueryParameters []string `json:"queryParameters"`
}

type CustomService struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	BaseURL   string           `json:"baseUrl"`
	AuthKind  string           `json:"authKind"`
	Header    string           `json:"header"`
	Headers   StaticHeaders    `json:"headers,omitempty"`
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

func (d CustomService) compile() (Service, error) {
	if err := d.validateMetadata(); err != nil {
		return Service{}, err
	}
	if err := d.validateAuth(); err != nil {
		return Service{}, err
	}
	if err := d.Headers.validate(d.Header); err != nil {
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
	if managedRequestHeader(strings.ToLower(d.Header)) {
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

func (d CustomService) definitionHash() string {
	d.Headers = d.Headers.canonical()
	d.Endpoints = append([]CustomEndpoint(nil), d.Endpoints...)
	for i := range d.Endpoints {
		d.Endpoints[i].Description = ""
		// Methods are a set; retain the legacy hash for GET-only definitions.
		methods := d.Endpoints[i].sortedMethods()
		if slices.Equal(methods, []string{"GET"}) {
			methods = nil
		}
		d.Endpoints[i].Methods = methods
	}
	data, _ := json.Marshal(d)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
