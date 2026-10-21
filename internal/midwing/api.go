package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
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
	return c.CallWithBody(ctx, service, method, path, nil)
}

// CallWithBody makes an authorized JSON API request. Request bodies are accepted only for POST.
func (c *Client) CallWithBody(ctx context.Context, service, method, path string, requestBody json.RawMessage) (json.RawMessage, error) {
	response, err := c.call(ctx, service, method, path, requestBody, true, ResponseOptions{}, nil)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(response), nil
}

func (c *Client) call(ctx context.Context, service, method, path string, requestBody json.RawMessage, requireJSON bool, options ResponseOptions, consume func([]byte) error) ([]byte, error) {
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxResponseBytes
	}
	if options.MaxBytes < 1 || options.MaxBytes == math.MaxInt64 {
		return nil, errors.New("response size limit must be between 1 and 9223372036854775806 bytes")
	}
	if options.Accept == "" {
		options.Accept = "*/*"
		if requireJSON {
			options.Accept = "application/json"
		}
	}
	mediaType, _, err := mime.ParseMediaType(options.Accept)
	if err != nil || !strings.Contains(mediaType, "/") || strings.ContainsAny(options.Accept, "\r\n") {
		return nil, errors.New("accept must be a valid media type")
	}
	method = strings.ToUpper(method)
	switch method {
	case "GET":
		if len(requestBody) != 0 {
			return nil, errors.New("GET requests must not include a body")
		}
	case "POST":
		if len(requestBody) == 0 || len(requestBody) > 1024*1024 || !json.Valid(requestBody) {
			return nil, errors.New("POST requires a valid JSON body of at most 1 MiB")
		}
	default:
		return nil, errors.New("only GET and POST are supported")
	}
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
	a := Activity{Time: time.Now().UTC(), Operation: service + " " + method + " " + operation, Outcome: "failed"}
	result, callErr := c.execute(ctx, s, state.Connections, permission, method, path, requestBody, options, &a)
	if callErr == nil && requireJSON && len(result) != 0 && !json.Valid(result) {
		result = nil
		callErr = errors.New("service returned invalid JSON")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		callErr = errors.Join(callErr, ctxErr)
	}
	if callErr == nil && consume != nil {
		callErr = consume(result)
	}
	if callErr == nil {
		callErr = ctx.Err()
	}
	if callErr == nil {
		a.Outcome = "success"
	}
	if e = c.Store.Record(a); e != nil {
		return nil, errors.Join(callErr, errors.New("could not record activity"))
	}
	return result, callErr
}

func (c *Client) execute(ctx context.Context, s Service, connections []Connection, permission, method, path string, requestBody json.RawMessage, options ResponseOptions, a *Activity) ([]byte, error) {
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
	var requestReader io.Reader
	if len(requestBody) != 0 {
		requestReader = bytes.NewReader(requestBody)
	}
	req, e := s.request(ctx, method, path, requestReader, token)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Accept", options.Accept)
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
	body, e := readResponse(resp, options.MaxBytes)
	if e != nil {
		return nil, e
	}
	if responseContainsSecret(body, token, previous) {
		return nil, errors.New("response contains credential; output suppressed")
	}
	return body, nil
}
