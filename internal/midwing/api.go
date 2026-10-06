package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

type Client struct {
	Store *Store
	HTTP  *http.Client
}

func (c *Client) Call(ctx context.Context, service, method, path string) (json.RawMessage, error) {
	state, e := c.Store.RegistrySnapshot()
	if e != nil {
		return nil, e
	}
	s, e := findService(state.Services, service)
	if e != nil {
		return nil, e
	}
	permission, operation, e := s.allowed(method, path)
	if e != nil {
		return nil, e
	}
	a := Activity{Time: time.Now().UTC(), Operation: service + " GET " + operation, Outcome: "failed"}
	result, callErr := c.execute(ctx, s, state.Connections, permission, path, &a)
	if e = c.Store.Record(a); e != nil {
		return nil, errors.Join(callErr, errors.New("could not record activity"))
	}
	return result, callErr
}
func (c *Client) execute(ctx context.Context, s Service, connections []Connection, permission, path string, a *Activity) (json.RawMessage, error) {
	var conn *Connection
	for i := range connections {
		if connections[i].Service == s.ID {
			conn = &connections[i]
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("connect %s in the desktop app first", s.Name)
	}
	if conn.DefinitionHash != s.DefinitionHash {
		return nil, errors.New("service definition changed; reconnect in the desktop app")
	}
	if !slices.Contains(conn.Permissions, permission) {
		a.Outcome = "denied"
		return nil, errors.New("operation is not allowed by connection permissions; grant the required permission in the desktop app")
	}
	token, e := c.Store.Vault.Get(conn.CredentialRef)
	if e != nil {
		return nil, errors.New("could not retrieve credential")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("credential is empty")
	}
	previous := token
	if s.Password != nil && s.Password.RefreshPath != "" {
		token, e = c.authRequest(ctx, s, s.Password.RefreshPath, nil, token)
		if e != nil {
			return nil, e
		}
		if e = c.Store.UpdateToken(*conn, previous, token); e != nil {
			return nil, e
		}
	}
	req, e := s.request(ctx, "GET", path, nil, token)
	if e != nil {
		return nil, e
	}
	resp, e := c.httpClient().Do(req)
	if e != nil {
		return nil, errors.New("service request failed")
	}
	defer resp.Body.Close()
	a.Status = resp.StatusCode
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("connection expired or unauthorized; reconnect in the desktop app")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned HTTP %d", s.Name, resp.StatusCode)
	}
	body, e := readJSON(resp, 8*1024*1024)
	if e != nil {
		return nil, e
	}
	if responseContainsSecret(body, token, previous) {
		return nil, errors.New("response contains credential; output suppressed")
	}
	a.Outcome = "success"
	return json.RawMessage(body), nil
}

// Inspect every decoded string, including object keys and duplicate-key values.
func responseContainsSecret(body []byte, secrets ...string) bool {
	if !json.Valid(body) {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return true
		}
		if value, ok := token.(string); ok {
			for _, secret := range secrets {
				if secret != "" && strings.Contains(value, secret) {
					return true
				}
			}
		}
	}
}
