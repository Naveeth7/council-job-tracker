package tracker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBatchParallelHostsKeepSameHostSequential(t *testing.T) {
	s := NewScanner()
	s.Delay = 0
	var mu sync.Mutex
	active := map[string]int{}
	peak := map[string]int{}
	total, globalPeak, started := 0, 0, 0
	gate := make(chan struct{})
	s.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/jobs" {
			mu.Lock()
			host := r.URL.Host
			active[host]++
			if active[host] > peak[host] {
				peak[host] = active[host]
			}
			total++
			if total > globalPeak {
				globalPeak = total
			}
			started++
			if started == 2 {
				close(gate)
			}
			mu.Unlock()
			select {
			case <-gate:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			mu.Lock()
			active[host]--
			total--
			mu.Unlock()
		}
		body := `<a href="/jobs/cook">Cook</a>`
		if r.URL.Path == "/robots.txt" {
			body = "User-agent: *\nAllow: /"
		} else if r.URL.Path == "/jobs/cook" {
			body = `<script type="application/ld+json">{"@type":"JobPosting","title":"Cook","description":"Prepare food"}</script>`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	cs := []Council{{Name: "One", URL: "https://a.example/jobs", Adapter: "myjobscotland", Confirmed: true}, {Name: "Two", URL: "https://a.example/jobs", Adapter: "myjobscotland", Confirmed: true}, {Name: "Three", URL: "https://b.example/jobs", Adapter: "myjobscotland", Confirmed: true}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := s.scanBatch(ctx, cs)
	if globalPeak < 2 || peak["a.example"] != 1 {
		t.Fatalf("global peak %d; host peaks %v", globalPeak, peak)
	}
	for i, r := range result {
		if r.check.Name != cs[i].Name || r.check.Status != "checked" || len(r.jobs) != 1 {
			t.Fatalf("%d: %+v", i, r)
		}
	}
}
