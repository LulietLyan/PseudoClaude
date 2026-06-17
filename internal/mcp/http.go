package mcp

import "net/http"

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	next := req.Clone(req.Context())
	for name, value := range h.headers {
		next.Header.Set(name, value)
	}
	return base.RoundTrip(next)
}
