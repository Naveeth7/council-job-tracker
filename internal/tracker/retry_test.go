package tracker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTransientPageRecoversOnThirdAttempt(t *testing.T) {
	s := NewScanner()
	s.Delay = 0
	calls := 0
	s.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := "ready"
		if r.URL.Path == "/robots.txt" {
			body = "User-agent: *\nAllow: /"
		} else {
			calls++
			if calls < 3 {
				status = 504
				body = "Gateway timeout"
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	b, e := s.data(context.Background(), "https://example.org/advert")
	if e != nil || string(b) != "ready" || calls != 3 {
		t.Fatalf("%q %v calls=%d", b, e, calls)
	}
}
func TestAccessDenialIsNotRetried(t *testing.T) {
	s := NewScanner()
	s.Delay = 0
	calls := 0
	s.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		status := 403
		body := "Denied"
		if r.URL.Path == "/robots.txt" {
			status = 200
			body = "User-agent: *\nAllow: /"
		} else {
			calls++
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	_, e := s.data(context.Background(), "https://example.org/advert")
	if e == nil || calls != 1 {
		t.Fatalf("%v calls=%d", e, calls)
	}
}
