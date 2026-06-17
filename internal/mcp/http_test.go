package mcp

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHeaderRoundTripper(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	req, err := http.NewRequest("GET", "http://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	rt := headerRoundTripper{base: base, headers: map[string]string{"Authorization": "Bearer token"}}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("original request mutated: %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
