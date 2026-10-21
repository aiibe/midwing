package midwing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// StaticHeaders are public service configuration, not a credential store.
type StaticHeaders map[string]string

// Reject null values and duplicate names before JSON decoding can discard them.
func (h *StaticHeaders) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("headers must be an object of strings")
	}
	headers := StaticHeaders{}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name := token.(string)
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate static header %q", name)
		}
		seen[key] = true
		value, err := decoder.Token()
		if err != nil {
			return err
		}
		text, ok := value.(string)
		if !ok {
			return errors.New("static header values must be strings")
		}
		headers[name] = text
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	*h = headers
	return nil
}

func (h StaticHeaders) validate(credentialHeader string) error {
	seen := map[string]bool{}
	for name, value := range h {
		if !headerName.MatchString(name) || len(name) > 80 {
			return errors.New("enter valid static header names (max 80 characters)")
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("duplicate static header %q", name)
		}
		seen[key] = true
		if strings.EqualFold(name, credentialHeader) || reservedStaticHeader(key) {
			return fmt.Errorf("static header %q is reserved for authentication or request handling", name)
		}
		for i := 0; i < len(value); i++ {
			if value[i] < 32 && value[i] != '\t' || value[i] == 127 {
				return fmt.Errorf("static header %q contains invalid control characters", name)
			}
		}
	}
	return nil
}

func managedRequestHeader(name string) bool {
	switch name {
	case "host", "content-length", "connection", "transfer-encoding", "cookie", "content-type", "accept", "user-agent":
		return true
	}
	return false
}

func reservedStaticHeader(name string) bool {
	if managedRequestHeader(name) {
		return true
	}
	switch name {
	case "authorization", "proxy-authorization", "keep-alive", "proxy-connection", "te",
		"trailer", "upgrade", "expect", "http2-settings", "accept-encoding":
		return true
	}
	return false
}

// Return a copy so compilation and hashing never mutate the source definition.
func (h StaticHeaders) canonical() StaticHeaders {
	if len(h) == 0 {
		return nil
	}
	out := StaticHeaders{}
	for name, value := range h {
		out[http.CanonicalHeaderKey(name)] = value
	}
	return out
}
