package tracker

import (
	"context"
	"errors"
	"fmt"
	"github.com/temoto/robotstxt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const userAgent = "CouncilJobTracker/1.0"

type Scanner struct {
	Client            *http.Client
	Delay             time.Duration
	MaxPages, MaxJobs int
	robots            map[string]*robotstxt.RobotsData
}

func NewScanner() *Scanner {
	return &Scanner{Client: &http.Client{Timeout: 25 * time.Second}, Delay: 500 * time.Millisecond, MaxPages: 20, MaxJobs: 300, robots: map[string]*robotstxt.RobotsData{}}
}
func (s *Scanner) allowed(ctx context.Context, raw string) error {
	u, e := url.Parse(raw)
	if e != nil || !safeURL(raw) {
		return errors.New("invalid HTTPS URL")
	}
	origin := u.Scheme + "://" + u.Host
	if _, ok := s.robots[origin]; !ok {
		req, _ := http.NewRequestWithContext(ctx, "GET", origin+"/robots.txt", nil)
		req.Header.Set("User-Agent", userAgent)
		res, e := s.Client.Do(req)
		if e != nil {
			return errors.New("could not check robots.txt")
		}
		defer res.Body.Close()
		b, e := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
		if e != nil {
			return e
		}
		if res.StatusCode != 404 && res.StatusCode != 200 {
			return errors.New("robots.txt unavailable; manual check required")
		}
		var data *robotstxt.RobotsData
		if res.StatusCode == 404 {
			data, e = robotstxt.FromStatusAndBytes(404, nil)
		} else {
			data, e = robotstxt.FromBytes(b)
		}
		if e != nil {
			return errors.New("could not parse robots.txt")
		}
		s.robots[origin] = data
	}
	if !s.robots[origin].TestAgent(u.RequestURI(), userAgent) {
		return errors.New("site robots.txt disallows this URL; manual check required")
	}
	return nil
}
func (s *Scanner) page(ctx context.Context, raw string) (*html.Node, error) {
	if e := s.allowed(ctx, raw); e != nil {
		return nil, e
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.Delay):
	}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", userAgent)
	res, e := s.Client.Do(req)
	if e != nil {
		return nil, errors.New("page request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("page returned HTTP %d", res.StatusCode)
	}
	return parseHTML(io.LimitReader(res.Body, 5*1024*1024))
}
func (s *Scanner) ScanCouncil(ctx context.Context, c Council) ([]Job, Check) {
	check := Check{Name: c.Name, URL: c.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(status, msg string) ([]Job, Check) {
		check.Status = status
		check.Message = msg
		return []Job{}, check
	}
	if c.URL == "" {
		return fail("not_configured", "Add a careers link to config/councils.csv.")
	}
	if !c.Confirmed {
		return fail("needs_verification", "Confirm the supplied link belongs to this council, then set confirmed=true.")
	}
	if c.Adapter != "myjobscotland" && c.Adapter != "talentsoft" {
		return fail("manual_check", "This recruitment platform needs a tested connector. Open the careers page for now.")
	}
	next := c.URL
	seenPages := map[string]bool{}
	seenJobs := map[string]bool{}
	jobURLs := []string{}
	partial := false
	expectedTotal := 0
	for page := 0; next != ""; page++ {
		if page >= s.MaxPages || seenPages[next] {
			partial = true
			break
		}
		seenPages[next] = true
		n, e := s.page(ctx, next)
		if e != nil {
			return fail("failed", e.Error())
		}
		ls := links(n, next, c.Adapter)
		if t := advertisedTotal(n); t > expectedTotal {
			expectedTotal = t
		}
		for _, j := range ls.jobs {
			if !seenJobs[j] {
				seenJobs[j] = true
				jobURLs = append(jobURLs, j)
			}
		}
		next = ls.next
	}
	if expectedTotal > len(jobURLs) {
		partial = true
	}
	if len(jobURLs) == 0 {
		return fail("needs_attention", "No readable vacancy links were found. The page may be empty, changed, or require JavaScript; zero jobs is not confirmed.")
	}
	if len(jobURLs) > s.MaxJobs {
		jobURLs = jobURLs[:s.MaxJobs]
		partial = true
	}
	out := []Job{}
	failures := 0
	for _, raw := range jobURLs {
		n, e := s.page(ctx, raw)
		if e != nil {
			failures++
			continue
		}
		jobs := parseJobs(n, raw, c.Name)
		if len(jobs) == 0 {
			failures++
			continue
		}
		check.Reviewed++
		for _, j := range jobs {
			out = append(out, j)
		}
	}
	check.Status = "checked"
	check.Message = fmt.Sprintf("Reviewed %d vacancy descriptions across all available categories; collected %d.", check.Reviewed, len(out))
	if failures > 0 || partial {
		check.Status = "partial"
		check.Message += fmt.Sprintf(" Incomplete coverage: %d descriptions unavailable; page/job limit reached: %t.", failures, partial)
	}
	return out, check
}
func (s *Scanner) Run(ctx context.Context, councils []Council, state *State, now time.Time) Report {
	report := Report{GeneratedAt: now.UTC().Format(time.RFC3339), Keywords: Keywords, Jobs: []Job{}, Checks: []Check{}}
	// Restrict publication to councils currently configured. Preserve historical jobs in private state.
	current := map[string]bool{}
	for _, c := range councils {
		current[c.Name] = true
		jobs, ch := s.ScanCouncil(ctx, c)
		report.Checks = append(report.Checks, ch)
		seen := map[string]bool{}
		for _, j := range jobs {
			seen[j.ID] = true
			if old, ok := state.Jobs[j.ID]; ok {
				j.FirstSeen = old.FirstSeen
			} else {
				j.FirstSeen = report.GeneratedAt
			}
			j.LastSeen = report.GeneratedAt
			j.Status = "listed"
			if IsClosed(j.Deadline, now) {
				j.Status = "closed"
			}
			state.Jobs[j.ID] = j
		}
		for id, j := range state.Jobs {
			if j.Council != c.Name || seen[id] {
				continue
			}
			if IsClosed(j.Deadline, now) {
				j.Status = "closed"
			} else if ch.Status == "checked" {
				j.Status = "not_listed"
			} else {
				j.Status = "unverified"
			}
			state.Jobs[id] = j
		}
	}
	for _, j := range state.Jobs {
		if current[j.Council] {
			report.Jobs = append(report.Jobs, j)
		}
	}
	sort.Slice(report.Jobs, func(i, j int) bool { return report.Jobs[i].FirstSeen > report.Jobs[j].FirstSeen })
	return report
}
func NormalizeName(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
