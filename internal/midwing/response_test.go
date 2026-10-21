package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawResponses(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		for _, tc := range []struct {
			name, body string
			status     int
			wantError  bool
		}{
			{"binary", "RIFF\x00\xffaudio", 200, false},
			{"empty", "", 204, false},
			{"JSON", `{"value":"safe"}`, 200, false},
			{"raw credential", "RIFF\x00secret-token\xff", 200, true},
			{"malformed JSON credential", `{"value":"secret-token`, 200, true},
			{"truncated escaped credential", `{"value":"secret\u002dtoken`, 200, true},
			{"JSON Lines escaped credential", "{\"value\":\"secret\\u002dtoken\"}\n{\"safe\":true}", 200, true},
			{"escape before malformed suffix", `{"value":"secret\u002dtoken\q`, 200, true},
			{"escaped JSON credential", `{"value":"secret\u002dtoken","value":"safe"}`, 200, true},
			{"unauthorized", "secret-token", 401, true},
			{"limit", strings.Repeat("a", 8*1024*1024), 200, false},
			{"oversized", strings.Repeat("a", 8*1024*1024+1), 200, true},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				store := testStore(t)
				d := tokenDefinition()
				d.Endpoints = []CustomEndpoint{{Path: "/audio", Methods: []string{"GET", "POST"}}}
				if err := store.CreateCustomService(d); err != nil {
					t.Fatal(err)
				}
				if err := store.SaveService(d.ID, "secret-token", []string{"endpoint-1", "endpoint-1-post"}); err != nil {
					t.Fatal(err)
				}
				client := Client{Store: store, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
					var body []byte
					var err error
					expectedBody := ""
					if method == "POST" {
						expectedBody = "{}"
					}
					if r.Body != nil {
						body, err = io.ReadAll(r.Body)
					}
					if err != nil || string(body) != expectedBody || r.Method != method || r.Header.Get("X-API-Key") != "secret-token" || r.Header.Get("Accept") != "*/*" {
						t.Fatal("incorrect authenticated POST request")
					}
					return &http.Response{StatusCode: tc.status, Body: ioBody(tc.body)}, nil
				})}}
				var out bytes.Buffer
				var requestBody json.RawMessage
				if method == "POST" {
					requestBody = json.RawMessage(`{}`)
				}
				err := client.CallTo(context.Background(), d.ID, method, "/audio", requestBody, &out, ResponseOptions{})
				if (err != nil) != tc.wantError {
					t.Fatalf("unexpected error: %v", err)
				}
				wantOutcome := "success"
				if tc.wantError {
					wantOutcome = "failed"
					if out.Len() != 0 {
						t.Fatal("failed response reached output")
					}
				} else if out.String() != tc.body {
					t.Fatal("response bytes changed")
				}
				history, err := store.History()
				if err != nil || len(history) != 1 || history[0].Outcome != wantOutcome {
					t.Fatalf("incorrect history: %+v (%v)", history, err)
				}
			})
		}
	}

}

type failingResponseReader struct{}

func (failingResponseReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func (failingResponseReader) Close() error { return nil }

type responseWriter func([]byte) (int, error)

func (f responseWriter) Write(p []byte) (int, error) { return f(p) }

func TestRawResponseFailuresAndOptions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		options     ResponseOptions
		reader      io.ReadCloser
		writer      io.Writer
		permissions []string
		wantError   string
		wantAccept  string
		wantCalls   int
	}{
		{name: "media type and larger limit", options: ResponseOptions{Accept: "audio/wav", MaxBytes: 9 << 20}, reader: ioBody(strings.Repeat("a", (8<<20)+1)), wantAccept: "audio/wav", wantCalls: 1},
		{name: "exact configured limit", options: ResponseOptions{MaxBytes: 4}, reader: ioBody("safe"), wantAccept: "*/*", wantCalls: 1},
		{name: "configured limit exceeded", options: ResponseOptions{MaxBytes: 3}, wantError: "size limit", wantCalls: 1},
		{name: "negative limit", options: ResponseOptions{MaxBytes: -1}, wantError: "size limit"},
		{name: "overflowing limit", options: ResponseOptions{MaxBytes: 1<<63 - 1}, wantError: "size limit"},
		{name: "invalid media type", options: ResponseOptions{Accept: "invalid"}, wantError: "media type"},
		{name: "header injection", options: ResponseOptions{Accept: "audio/wav\r\nX-Test: bad"}, wantError: "media type"},
		{name: "read failure", reader: failingResponseReader{}, wantError: "could not read", wantCalls: 1},
		{name: "write failure", writer: responseWriter(func([]byte) (int, error) { return 0, errors.New("disk full") }), wantError: "disk full", wantCalls: 1},
		{name: "short write", writer: responseWriter(func(p []byte) (int, error) { return len(p) - 1, nil }), wantError: io.ErrShortWrite.Error(), wantCalls: 1},
		{name: "denied", permissions: []string{"endpoint-1-post"}, wantError: "permissions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := testStore(t)
			d := tokenDefinition()
			d.Endpoints = []CustomEndpoint{{Path: "/audio", Methods: []string{"GET", "POST"}}}
			if err := store.CreateCustomService(d); err != nil {
				t.Fatal(err)
			}
			permissions := tc.permissions
			if permissions == nil {
				permissions = []string{"endpoint-1"}
			}
			if err := store.SaveService(d.ID, "secret-token", permissions); err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := Client{Store: store, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.wantAccept != "" && r.Header.Get("Accept") != tc.wantAccept {
					t.Fatalf("unexpected Accept: %q", r.Header.Get("Accept"))
				}
				reader := tc.reader
				if reader == nil {
					reader = ioBody("safe")
				}
				return &http.Response{StatusCode: 200, Body: reader}, nil
			})}}
			var out bytes.Buffer
			writer := tc.writer
			if writer == nil {
				writer = &out
			}
			err := client.CallTo(context.Background(), d.ID, "GET", "/audio", nil, writer, tc.options)
			if tc.wantError == "" {
				if err != nil || out.Len() == 0 {
					t.Fatalf("missing response: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) || out.Len() != 0 {
				t.Fatalf("unexpected output or error: %v", err)
			}
			if calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tc.wantCalls)
			}
			history, err := store.History()
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantCalls != 0 || tc.name == "denied" {
				wantOutcome := "success"
				if tc.wantError != "" {
					wantOutcome = "failed"
				}
				if tc.name == "denied" {
					wantOutcome = "denied"
				}
				if len(history) != 1 || history[0].Outcome != wantOutcome {
					t.Fatalf("unexpected history: %+v", history)
				}
			}
		})
	}
	if err := (&Client{}).CallTo(context.Background(), "", "GET", "/", nil, nil, ResponseOptions{}); err == nil {
		t.Fatal("accepted nil writer")
	}
}

func TestResponseConsumerOutcome(t *testing.T) {
	for _, outcome := range []string{"close failure", "canceled request", "canceled after write", "record failure"} {
		t.Run(outcome, func(t *testing.T) {
			store := testStore(t)
			if err := store.SaveService("sample-api", "secret-token", []string{"profile"}); err != nil {
				t.Fatal(err)
			}
			if outcome == "record failure" {
				if err := os.Mkdir(filepath.Join(store.Dir, "activity.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := Client{Store: store, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				if outcome == "canceled request" {
					cancel()
					return nil, context.Canceled
				}
				return &http.Response{StatusCode: 200, Body: ioBody("safe")}, nil
			})}}
			calls := 0
			err := client.CallWithResponse(ctx, "sample-api", "GET", "/profile", nil, ResponseOptions{}, func(body []byte) error {
				calls++
				if string(body) != "safe" {
					t.Fatal("unexpected response")
				}
				switch outcome {
				case "close failure":
					return errors.New("close failed")
				case "canceled after write":
					cancel()
				}
				return nil
			})
			wantCalls := 1
			if outcome == "canceled request" {
				wantCalls = 0
			}
			if err == nil || calls != wantCalls {
				t.Fatalf("output failure was lost: %v (%d calls)", err, calls)
			}
			if outcome == "record failure" {
				if !strings.Contains(err.Error(), "could not record activity") {
					t.Fatal(err)
				}
				return
			}
			history, historyErr := store.History()
			if historyErr != nil || len(history) != 1 || history[0].Outcome != "failed" {
				t.Fatalf("unexpected history: %+v (%v)", history, historyErr)
			}
			if strings.HasPrefix(outcome, "canceled") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestJSONOutputFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		writer     io.Writer
		wantError  string
	}{
		{name: "broken pipe", body: `{}`, writer: responseWriter(func([]byte) (int, error) { return 0, errors.New("broken pipe") }), wantError: "broken pipe"},
		{name: "short write", body: `{}`, writer: responseWriter(func(p []byte) (int, error) { return len(p) - 1, nil }), wantError: io.ErrShortWrite.Error()},
		{name: "invalid JSON", body: "invalid", wantError: "invalid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := testStore(t)
			if err := store.SaveService("sample-api", "secret-token", []string{"profile"}); err != nil {
				t.Fatal(err)
			}
			client := Client{Store: store, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Accept") != "application/json" {
					t.Fatal("incorrect JSON Accept header")
				}
				return &http.Response{StatusCode: 200, Body: ioBody(tc.body)}, nil
			})}}
			writer := tc.writer
			if writer == nil {
				writer = responseWriter(func([]byte) (int, error) { t.Fatal("invalid JSON reached stdout"); return 0, nil })
			}
			err := client.CallJSONTo(context.Background(), "sample-api", "GET", "/profile", nil, writer)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatal(err)
			}
			history, err := store.History()
			if err != nil || len(history) != 1 || history[0].Outcome != "failed" {
				t.Fatalf("unexpected history: %+v (%v)", history, err)
			}
		})
	}
}
