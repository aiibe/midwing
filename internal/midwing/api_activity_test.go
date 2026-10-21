package midwing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestOversizedActivityRejectedBeforeRequest(t *testing.T) {
	for _, output := range []string{"return JSON", "print JSON", "download"} {
		t.Run(output, func(t *testing.T) {
			s := testStore(t)
			d := tokenDefinition()
			d.Endpoints = []CustomEndpoint{{Path: "/items/{id}", Methods: []string{"POST"}}}
			if err := s.CreateCustomService(d); err != nil {
				t.Fatal(err)
			}
			if err := saveTestService(s, d.ID, "secret-token", []string{"endpoint-1-post"}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := Client{Store: s, HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 201, Body: ioBody(`{"created":true}`)}, nil
			})}}
			path := "/items/" + strings.Repeat("x", activityRecordLimit)
			var out bytes.Buffer
			var err error
			switch output {
			case "return JSON":
				var body json.RawMessage
				body, err = client.CallWithBody(context.Background(), d.ID, "POST", path, json.RawMessage(`{}`))
				out.Write(body)
			case "print JSON":
				err = client.CallJSONTo(context.Background(), d.ID, "POST", path, json.RawMessage(`{}`), &out)
			case "download":
				err = client.CallTo(context.Background(), d.ID, "POST", path, json.RawMessage(`{}`), &out, ResponseOptions{})
			}
			if err == nil || !strings.Contains(err.Error(), "activity record exceeds size limit") {
				t.Fatalf("expected activity size rejection, got %v", err)
			}
			if calls != 0 || out.Len() != 0 {
				t.Fatalf("oversized operation reached service or output: calls=%d, output=%q", calls, out.String())
			}
			rows, err := s.History()
			if err != nil || len(rows) != 0 {
				t.Fatalf("rejected request changed history: %v, %v", rows, err)
			}
		})
	}
}
