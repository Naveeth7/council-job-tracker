package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"strings"
	"time"
)

func vacancyDate(s string) string {
	s = strings.TrimSpace(strip(s))
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2/1/2006", "02 Jan 2006", "2 January 2006", time.RFC3339} {
		if d, e := time.Parse(layout, s); e == nil {
			return d.Format("2006-01-02")
		}
	}
	if len(s) > 10 {
		if d, e := time.Parse("2006-01-02", s[:10]); e == nil {
			return d.Format("2006-01-02")
		}
	}
	return ""
}
func firstValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strip(str(m[k])); v != "" {
			return v
		}
	}
	return ""
}
func (s *Scanner) scanITrent(ctx context.Context, c Council) ([]Job, Check) {
	ch := Check{Name: c.Name, URL: c.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(msg string) ([]Job, Check) {
		ch.Status = "failed"
		if strings.Contains(msg, "security challenge") {
			ch.Status = "blocked"
		}
		ch.Message = msg
		return nil, ch
	}
	n, e := s.page(ctx, c.URL)
	if e != nil {
		return fail(e.Error())
	}
	base, _ := url.Parse(c.URL)
	params := url.Values{}
	for key, values := range base.Query() {
		if strings.EqualFold(key, "WVID") {
			params.Set("WVID", values[0])
		}
	}
	// The public search page creates an anonymous session. Keep it in memory only.
	walk(n, func(x *html.Node) {
		if x.Data != "form" || !strings.EqualFold(attr(x, "id"), "ETREC179GF") {
			return
		}
		action, err := url.Parse(attr(x, "action"))
		if err != nil {
			return
		}
		for _, key := range []string{"WVID", "USESSION", "LANG"} {
			if v := action.Query().Get(key); v != "" {
				params.Set(key, v)
			}
		}
	})
	if params.Get("WVID") == "" || params.Get("USESSION") == "" {
		return fail("Public portal session could not be initialised; no vacancies were confirmed.")
	}
	endpoint := func(name string, q url.Values) string {
		ref := &url.URL{Path: name, RawQuery: q.Encode()}
		return base.ResolveReference(ref).String()
	}
	data, e := s.data(ctx, endpoint("etrec106gf.json", params))
	if e != nil {
		return fail(e.Error())
	}
	var listing struct {
		Results []map[string]any `json:"results"`
		Search  struct {
			Total int `json:"total_rec"`
			From  int `json:"rec_from"`
			To    int `json:"rec_to"`
		} `json:"search"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields["results"] == nil || fields["search"] == nil {
		return fail("Public vacancy search returned an unexpected response.")
	}
	if json.Unmarshal(data, &listing) != nil {
		return fail("Public vacancy search returned unreadable data.")
	}
	if len(listing.Results) == 0 && listing.Search.Total != 0 {
		return fail("Vacancy count was advertised but no records were readable.")
	}
	out := []Job{}
	failures := 0
	seen := map[string]bool{}
	for _, row := range listing.Results {
		id := str(row["vacancy_id"])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		q := url.Values{}
		for k, v := range params {
			q[k] = append([]string{}, v...)
		}
		q.Set("VACANCY_ID", id)
		b, err := s.data(ctx, endpoint("etrec107gf.json", q))
		if err != nil {
			failures++
			continue
		}
		var details []map[string]any
		if json.Unmarshal(b, &details) != nil || len(details) == 0 {
			failures++
			continue
		}
		detail := details[0]
		title := firstValue(row, "job_title")
		if title == "" {
			title = firstValue(detail, "job_titles", "job_title")
		}
		desc := firstValue(detail, "job_description")
		if title == "" || desc == "" {
			failures++
			continue
		}
		publicParams := url.Values{"WVID": {params.Get("WVID")}, "VACANCY_ID": {id}}
		raw := endpoint("ETREC107GF.open", publicParams)
		sum := sha256.Sum256([]byte(c.Name + "|itrent:" + id))
		posted := vacancyDate(str(detail["vacancy_d"]))
		if posted == "" {
			posted = vacancyDate(str(detail["date_posted"]))
		}
		j := Job{ID: hex.EncodeToString(sum[:12]), Council: c.Name, Employer: c.Name, Title: title, URL: raw, Description: desc, Posted: posted, Deadline: vacancyDate(firstValue(detail, "app_close_d", "app_clo_d")), Location: firstValue(detail, "location"), Salary: firstValue(detail, "salary"), Matched: Matches(title + " " + desc), Sponsorship: "Not stated — verify with employer"}
		if j.Deadline == "" {
			j.Deadline = vacancyDate(str(row["app_close_d"]))
		}
		for _, phrase := range []string{"unable to offer visa sponsorship", "cannot offer visa sponsorship", "do not offer visa sponsorship", "sponsorship is not available for this post", "not eligible for sponsorship"} {
			if strings.Contains(strings.ToLower(desc), phrase) {
				j.Sponsorship = "Advert says no visa sponsorship"
			}
		}
		out = append(out, j)
		ch.Reviewed++
	}
	ch.Status = "checked"
	ch.Message = fmt.Sprintf("Reviewed %d full vacancy descriptions from the public portal API across all categories; advertised %d.", ch.Reviewed, listing.Search.Total)
	if failures > 0 || listing.Search.Total > len(out) {
		ch.Status = "partial"
		ch.Message += fmt.Sprintf(" Incomplete coverage: %d descriptions unavailable or additional records not retrieved.", failures)
	}
	return out, ch
}
