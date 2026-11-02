package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Greater Jobs publishes its anonymous search configuration in the page and
// uses a publishable browser key. This key is discovered afresh, never stored.
func (s *Scanner) scanGreater(ctx context.Context, c Council) ([]Job, Check) {
	ch := Check{Name: c.Name, URL: c.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(msg string) ([]Job, Check) { ch.Status = "failed"; ch.Message = msg; return nil, ch }
	b, err := s.data(ctx, c.URL)
	if err != nil {
		return fail(err.Error())
	}
	key := regexp.MustCompile(`pk_[a-zA-Z0-9]+`).FindString(string(b))
	if key == "" {
		return fail("The public job-search configuration was not found")
	}
	urls := []string{}
	deadlines := map[string]string{}
	seen := map[string]bool{}
	total := -1
	complete := false
	for page := 1; page <= s.MaxPages; page++ {
		// Employer, not job category, scopes this shared board to Blackpool Council.
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"10"}, "query": {""}, "filters[custom_data.is_internal]": {"false"}, "filters[company_name][0]": {"Blackpool Council"}}
		data, e := s.requestData(ctx, "https://api.inploi.com/search/results?"+q.Encode(), nil, map[string]string{"x-publishable-key": key})
		if e != nil {
			return fail(e.Error())
		}
		var r struct {
			Data []struct {
				ID      int    `json:"id"`
				Company string `json:"company_name"`
				Custom  struct {
					Expiry string `json:"expiryDate"`
				} `json:"custom_data"`
			} `json:"data"`
			Pagination *struct {
				Total int `json:"total"`
				Last  int `json:"last_page"`
			} `json:"pagination"`
		}
		if json.Unmarshal(data, &r) != nil || r.Pagination == nil {
			return fail("Public job search returned an unexpected response")
		}
		total = r.Pagination.Total
		added := 0
		for _, j := range r.Data {
			if j.ID <= 0 || j.Company != "Blackpool Council" {
				continue
			}
			u := "https://www.greater.jobs/job/" + strconv.Itoa(j.ID)
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
				deadlines[u] = vacancyDate(j.Custom.Expiry)
				added++
			}
		}
		if len(urls) >= total {
			complete = true
			break
		}
		if added == 0 || len(urls) >= s.MaxJobs {
			break
		}
	}
	out := []Job{}
	failures := 0
	for _, raw := range urls {
		n, e := s.page(ctx, raw)
		if e != nil {
			failures++
			continue
		}
		js := parseJobs(n, raw, c.Name)
		if len(js) != 1 || !strings.EqualFold(js[0].Employer, "Blackpool Council") {
			failures++
			continue
		}
		if js[0].Deadline == "" {
			js[0].Deadline = deadlines[raw]
		}
		out = append(out, js[0])
		ch.Reviewed++
	}
	ch.Status = "checked"
	ch.Message = fmt.Sprintf("Reviewed %d full descriptions; public employer search advertises %d Blackpool Council vacancies across all categories.", len(out), total)
	if !complete || failures > 0 {
		ch.Status = "partial"
		ch.Message += " Some search pages or descriptions were unavailable."
	}
	return out, ch
}
