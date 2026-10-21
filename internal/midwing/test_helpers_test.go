package midwing

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type memoryVault map[string]string

func (v memoryVault) Set(k, s string) error { v[k] = s; return nil }

func (v memoryVault) Get(k string) (string, error) {
	s, ok := v[k]
	if !ok {
		return "", errors.New("missing")
	}
	return s, nil
}

func (v memoryVault) Delete(k string) error { delete(v, k); return nil }

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testStore(t *testing.T) *Store {
	registerTokenFixture(t)
	return &Store{Dir: t.TempDir(), Vault: memoryVault{}}
}

type stringBody struct{ *strings.Reader }

func (stringBody) Close() error { return nil }

func ioBody(s string) stringBody { return stringBody{strings.NewReader(s)} }
