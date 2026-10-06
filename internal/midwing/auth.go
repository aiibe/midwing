package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

func (c *Client) httpClient() *http.Client {
	client := http.Client{Timeout: 30 * time.Second}
	if c.HTTP != nil {
		client = *c.HTTP
		if client.Timeout == 0 {
			client.Timeout = 30 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}
func (s Service) request(ctx context.Context, method, path string, body io.Reader, token string) (*http.Request, error) {
	origin, e := url.Parse(s.Origin)
	if e != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return nil, errors.New("invalid service origin")
	}
	target, e := url.Parse(path)
	if e != nil || target.IsAbs() || target.Host != "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, errors.New("invalid service endpoint")
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(s.Origin, "/")+path, body)
	if e != nil {
		return nil, errors.New("could not create request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Midwing/0.1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(s.Header, s.Prefix+token)
	}
	return req, nil
}
func readJSON(resp *http.Response, limit int64) ([]byte, error) {
	body, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return nil, errors.New("could not read service response")
	}
	if int64(len(body)) > limit {
		return nil, errors.New("service response exceeds size limit")
	}
	if !json.Valid(body) {
		return nil, errors.New("service returned invalid JSON")
	}
	return body, nil
}
func extractToken(body []byte, path string) (string, error) {
	var value any
	if e := json.Unmarshal(body, &value); e != nil {
		return "", errors.New("invalid authentication response")
	}
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return "", errors.New("authentication response has no token")
		}
		value = object[part]
	}
	token, ok := value.(string)
	if !ok || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("authentication response has no valid token")
	}
	return token, nil
}
func (c *Client) authRequest(ctx context.Context, s Service, path string, body io.Reader, token string) (string, error) {
	req, e := s.request(ctx, "POST", path, body, token)
	if e != nil {
		return "", e
	}
	resp, e := c.httpClient().Do(req)
	if e != nil {
		return "", errors.New("authentication request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("authentication failed (HTTP %d); reconnect in the desktop app", resp.StatusCode)
	}
	data, e := readJSON(resp, 1024*1024)
	if e != nil {
		return "", e
	}
	return extractToken(data, s.Password.TokenField)
}

// ConnectPassword uses the service's JSON protocol. The password is never persisted.
func (c *Client) ConnectPassword(ctx context.Context, service, email, password string, permissions []string) error {
	s, e := c.Store.Service(service)
	if e != nil {
		return e
	}
	return c.connectPassword(ctx, s, email, password, permissions)
}
func (c *Client) connectPassword(ctx context.Context, s Service, email, password string, permissions []string) error {
	if s.Password == nil {
		return errors.New("service does not support email/password authentication")
	}
	if e := validatePermissions(s, permissions); e != nil {
		return e
	}
	email = strings.TrimSpace(email)
	address, e := mail.ParseAddress(email)
	if e != nil || address.Address != email {
		return errors.New("enter a valid email address")
	}
	if password == "" {
		return errors.New("enter a password")
	}
	payload, e := json.Marshal(map[string]string{s.Password.EmailField: email, s.Password.PasswordField: password})
	if e != nil {
		return errors.New("could not create login request")
	}
	token, e := c.authRequest(ctx, s, s.Password.LoginPath, bytes.NewReader(payload), "")
	if e != nil {
		return e
	}
	// Reject accidental credential echoes from an incorrectly configured service.
	if token == password {
		return errors.New("authentication returned a password instead of a token")
	}
	return c.Store.saveToken(s.ID, token, permissions, &s)
}
