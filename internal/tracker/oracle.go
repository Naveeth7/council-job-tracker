package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const oracleAPI = "/hcmRestApi/resources/11.13.18.05/"

var oracleSitePattern = regexp.MustCompile(`/sites/([A-Za-z0-9_]+)(?:/|$)`)
var advertisedPounds = regexp.MustCompile(`£\s?\d[\d,]*(?:\.\d+)?(?:\s*[-–]\s*£?\s?\d[\d,]*(?:\.\d+)?)?`)

type oracleVacancy struct {
	ID               string `json:"Id"`
	Title            string `json:"Title"`
	Posted           string `json:"PostedDate"`
	Deadline         string `json:"PostingEndDate"`
	Location         string `json:"PrimaryLocation"`
	Description      string `json:"ExternalDescriptionStr"`
	Qualifications   string `json:"ExternalQualificationsStr"`
	Responsibilities string `json:"ExternalResponsibilitiesStr"`
	DetailPosted     string `json:"ExternalPostedStartDate"`
	DetailDeadline   string `json:"ExternalPostedEndDate"`
}

type oracleSearch struct {
	Items []struct {
		Site  string          `json:"SiteNumber"`
		Total *int            `json:"TotalJobsCount"`
		Jobs  []oracleVacancy `json:"requisitionList"`
	} `json:"items"`
}

func oracleJob(c Council, base *url.URL, site string, summary, detail oracleVacancy) (Job, bool) {
	if detail.ID != summary.ID {
		return Job{}, false
	}
	title := strip(detail.Title)
	if title == "" {
		title = strip(summary.Title)
	}
	parts := []string{}
	seen := map[string]bool{}
	for _, raw := range []string{detail.Description, detail.Qualifications, detail.Responsibilities} {
		if value := strip(raw); value != "" && !seen[value] {
			parts = append(parts, value)
			seen[value] = true
		}
	}
	desc := strings.Join(parts, " ")
	if title == "" || desc == "" || strip(detail.Description) == "" {
		return Job{}, false
	}
	posted := vacancyDate(detail.DetailPosted)
	if posted == "" {
		posted = vacancyDate(summary.Posted)
	}
	deadline := vacancyDate(detail.DetailDeadline)
	if deadline == "" {
		deadline = vacancyDate(summary.Deadline)
	}
	location := strip(detail.Location)
	if location == "" {
		location = strip(summary.Location)
	}
	sum := sha256.Sum256([]byte(c.Name + "|oracle:" + site + ":" + summary.ID))
	jobURL := *base
	jobURL.Path = strings.TrimRight(base.Path, "/") + "/job/" + url.PathEscape(summary.ID)
	jobURL.RawQuery = ""
	jobURL.Fragment = ""
	j := Job{ID: hex.EncodeToString(sum[:12]), Council: c.Name, Employer: c.Name, Title: title, URL: jobURL.String(), Description: desc, Location: location, Posted: posted, Deadline: deadline, Salary: advertisedPounds.FindString(desc), Matched: Matches(title + " " + desc), Sponsorship: "Not stated — verify with employer"}
	for _, phrase := range []string{"unable to offer visa sponsorship", "cannot offer visa sponsorship", "do not offer visa sponsorship", "not eligible for sponsorship", "sponsorship is not available for this post", "sponsorship is not available for this role"} {
		if strings.Contains(strings.ToLower(desc), phrase) {
			j.Sponsorship = "Advert says no visa sponsorship"
		}
	}
	return j, true
}

// Read the career site's own public search and detail resources without category
// or keyword restrictions. Each full description is matched locally afterwards.
func (s *Scanner) scanOracle(ctx context.Context, c Council) ([]Job, Check) {
	ch := Check{Name: c.Name, URL: c.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(msg string) ([]Job, Check) { ch.Status = "failed"; ch.Message = msg; return nil, ch }
	base, e := url.Parse(c.URL)
	if e != nil || !safeURL(c.URL) {
		return fail("Invalid Oracle careers URL.")
	}
	match := oracleSitePattern.FindStringSubmatch(base.Path)
	if len(match) != 2 {
		return fail("Oracle career site number could not be identified.")
	}
	site := match[1]
	// The configured source is the site root, not a particular expired advert.
	base.Path = base.Path[:strings.Index(base.Path, "/sites/")+len("/sites/")] + site
	api := func(resource, finder, expand string) string {
		u := base.ResolveReference(&url.URL{Path: oracleAPI + resource})
		q := url.Values{"onlyData": {"true"}, "finder": {finder}}
		if expand != "" {
			q.Set("expand", expand)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	summaries := []oracleVacancy{}
	seen := map[string]bool{}
	expected := -1
	partial := false
	offset := 0
	for page := 0; ; page++ {
		if page >= s.MaxPages || len(summaries) >= s.MaxJobs {
			partial = true
			break
		}
		raw, e := s.data(ctx, api("recruitingCEJobRequisitions", fmt.Sprintf("findReqs;siteNumber=%s,limit=25,offset=%d", site, offset), "requisitionList"))
		if e != nil {
			if len(summaries) == 0 {
				return fail(e.Error())
			}
			partial = true
			break
		}
		var response oracleSearch
		if json.Unmarshal(raw, &response) != nil || len(response.Items) != 1 || response.Items[0].Site != site || response.Items[0].Total == nil {
			return fail("Oracle vacancy search returned an unexpected response; coverage is not confirmed.")
		}
		item := response.Items[0]
		if *item.Total < 0 {
			return fail("Oracle vacancy total was invalid.")
		}
		if *item.Total > expected {
			expected = *item.Total
		}
		before := len(summaries)
		for _, job := range item.Jobs {
			if job.ID == "" || seen[job.ID] {
				continue
			}
			seen[job.ID] = true
			summaries = append(summaries, job)
			if len(summaries) >= s.MaxJobs {
				break
			}
		}
		if len(summaries) >= expected {
			break
		}
		if len(summaries) == before || len(item.Jobs) == 0 {
			partial = true
			break
		}
		offset += len(item.Jobs)
	}
	jobs := []Job{}
	failures := 0
	for _, summary := range summaries {
		raw, e := s.data(ctx, api("recruitingCEJobRequisitionDetails", "ById;Id="+summary.ID+",siteNumber="+site, ""))
		if e != nil {
			failures++
			continue
		}
		var response struct {
			Items []oracleVacancy `json:"items"`
		}
		if json.Unmarshal(raw, &response) != nil || len(response.Items) != 1 {
			failures++
			continue
		}
		j, ok := oracleJob(c, base, site, summary, response.Items[0])
		if !ok {
			failures++
			continue
		}
		jobs = append(jobs, j)
		ch.Reviewed++
	}
	ch.Status = "checked"
	ch.Message = "Read " + strconv.Itoa(ch.Reviewed) + " full vacancy descriptions from the council's direct Oracle careers site; advertised " + strconv.Itoa(expected) + " across all categories."
	if partial || failures > 0 || len(jobs) < expected {
		ch.Status = "partial"
		ch.Message += fmt.Sprintf(" Incomplete coverage: %d detail failures; %d unique listings retrieved.", failures, len(summaries))
	}
	return jobs, ch
}
