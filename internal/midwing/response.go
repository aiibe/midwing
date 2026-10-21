package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"unicode/utf8"
)

const DefaultMaxResponseBytes int64 = 8 * 1024 * 1024

// ResponseOptions controls raw response negotiation and the buffered size limit.
// Zero values accept any media type and use DefaultMaxResponseBytes.
type ResponseOptions struct {
	Accept   string
	MaxBytes int64
}

// CallJSONTo prints one checked JSON response and records output failures.
// Empty successful responses print nothing.
func (c *Client) CallJSONTo(ctx context.Context, service, method, path string, requestBody json.RawMessage, out io.Writer) error {
	if out == nil {
		return errors.New("output writer is required")
	}
	_, err := c.call(ctx, service, method, path, requestBody, true, ResponseOptions{}, func(response []byte) error {
		if len(response) == 0 {
			return nil
		}
		n, err := fmt.Fprintln(out, string(response))
		if err == nil && n != len(response)+1 {
			return io.ErrShortWrite
		}
		return err
	})
	return err
}

// CallWithBodyTo sends an authorized JSON POST and writes its successful response bytes to out.
func (c *Client) CallWithBodyTo(ctx context.Context, service, path string, requestBody json.RawMessage, out io.Writer) error {
	return c.CallTo(ctx, service, "POST", path, requestBody, out, ResponseOptions{})
}

// CallTo writes a checked GET or POST response, preserving its original bytes.
// The response is buffered and checked before any bytes reach out.
func (c *Client) CallTo(ctx context.Context, service, method, path string, requestBody json.RawMessage, out io.Writer, options ResponseOptions) error {
	if out == nil {
		return errors.New("output writer is required")
	}
	return c.CallWithResponse(ctx, service, method, path, requestBody, options, func(response []byte) error {
		_, err := io.Copy(out, bytes.NewReader(response))
		return err
	})
}

// CallWithResponse passes checked raw bytes to consume and records its outcome.
// The consumer must finish its output, including closing files, before returning.
func (c *Client) CallWithResponse(ctx context.Context, service, method, path string, requestBody json.RawMessage, options ResponseOptions, consume func([]byte) error) error {
	if consume == nil {
		return errors.New("response consumer is required")
	}
	_, err := c.call(ctx, service, method, path, requestBody, false, options, consume)
	return err
}

// Decode JSON escapes even in incomplete JSON, JSON Lines, or other response data.
// Match surrogate pairs together so their decoded UTF-8 representation is preserved.
var responseEscape = regexp.MustCompile(`\\(?:u[dD][89aAbB][0-9a-fA-F]{2}\\u[dD][c-fC-F][0-9a-fA-F]{2}|u[0-9a-fA-F]{4}|["\\/bfnrt])`)

func responseContainsSecret(body []byte, secrets ...string) bool {
	containsSecret := func(data []byte) bool {
		for _, secret := range secrets {
			if secret != "" && bytes.Contains(data, []byte(secret)) {
				return true
			}
		}
		return false
	}
	if containsSecret(body) {
		return true
	}
	decoded := responseEscape.ReplaceAllFunc(body, func(escape []byte) []byte {
		quoted := append([]byte{'"'}, escape...)
		quoted = append(quoted, '"')
		var value string
		if err := json.Unmarshal(quoted, &value); err != nil {
			return escape
		}
		return []byte(value)
	})
	// JSON decoders replace each invalid UTF-8 byte with the replacement rune.
	if !utf8.Valid(decoded) {
		decoded = bytes.Map(func(r rune) rune { return r }, decoded)
	}
	return containsSecret(decoded)
}
