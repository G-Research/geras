package useragent

import (
	"net/http"
)

type UserAgentTransport struct {
	base      http.RoundTripper
	customUA  string
}

func NewUserAgentTransport(base http.RoundTripper, customUA string) *UserAgentTransport {
	return &UserAgentTransport{
		base:     base,
		customUA: customUA,
	}
}

func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defaultUA := req.Header.Get("User-Agent")
	if defaultUA != "" {
		req.Header.Set("User-Agent", defaultUA+" "+t.customUA)
	} else {
		req.Header.Set("User-Agent", t.customUA)
	}
	return t.base.RoundTrip(req)
}
