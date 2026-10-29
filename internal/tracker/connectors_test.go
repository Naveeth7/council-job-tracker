package tracker

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureScanner(pages map[string]string) *Scanner {
	s := NewScanner()
	s.Delay = 0
	s.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		body, ok := pages[r.URL.RequestURI()]
		status := 200
		if r.URL.Path == "/robots.txt" {
			body = "User-agent: *\nAllow: /"
			ok = true
		}
		if !ok {
			status = 404
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	return s
}
func TestJobtrainPaginationAndAllRoles(t *testing.T) {
	s := fixtureScanner(map[string]string{
		"/jobs":                  `<input id="requestUrl" data-request-url="/cards">`,
		"/cards?Skip=0":          `<input id="totalMatchRecords" value="2"><input id="totalCurrentRecords" value="1"><a href="/Job/JobDetail?JobId=1">Cook</a>`,
		"/cards?Skip=1":          `<input id="totalMatchRecords" value="2"><input id="totalCurrentRecords" value="1"><a href="/Job/JobDetail?JobId=2">Digital Officer</a>`,
		"/Job/JobDetail?JobId=1": `<script type="application/ld&#x2B;json">{"@type":"JobPosting","title":"Cook","description":"Prepare meals","baseSalary":"&#xA3;25,000","hiringOrganization":{"name":"Example Council"}}</script>`,
		"/Job/JobDetail?JobId=2": `<script type="application/ld+json">{"@type":"JobPosting","title":"Digital Officer","description":"Support websites","datePosted":"2026-10-29","validThrough":"2026-11-12"}</script>`,
	})
	jobs, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/jobs", Adapter: "jobtrain", Confirmed: true})
	if ch.Status != "checked" || len(jobs) != 2 || jobs[0].Salary != "£25,000" || len(jobs[0].Matched) != 0 || jobs[1].Deadline != "2026-11-12" {
		t.Fatalf("%+v %+v", ch, jobs)
	}
}
func TestJobtrainMissingPageIsPartial(t *testing.T) {
	s := fixtureScanner(map[string]string{
		"/cards?Skip=0":          `<input id="totalMatchRecords" value="2"><input id="totalCurrentRecords" value="1"><a href="/Job/JobDetail?JobId=1">Cook</a>`,
		"/Job/JobDetail?JobId=1": `<script type="application/ld+json">{"@type":"JobPosting","title":"Cook","description":"Prepare meals"}</script>`,
	})
	jobs, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/cards?Skip=0", Adapter: "jobtrain", Confirmed: true})
	if ch.Status != "partial" || len(jobs) != 1 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
}
func TestHireserveFeedAndInvalidStructuredData(t *testing.T) {
	s := fixtureScanner(map[string]string{
		"/jobs": `<form action="/ic_job_feeds.feed_engine"><input type="hidden" name="p_web_site_id" value="6700"></form>`,
		"/ic_job_feeds.feed_engine?p_summary=Y&p_web_site_id=6700": `{"total":1,"jobs":[{"title":"Cook","weblink":"/vacancy/cook.html","publication":{"internet":{"publish_date":"2026-10-29 00:00:00","closing_date":"2026-11-12 00:00:00"}},"classifications":{"salary":{"name":"Salary Min","values":[{"class_val":"£25,000"}]}}}]}`,
		"/vacancy/cook.html": `<nav>Digital</nav><div class="job_description"><h1>Cook</h1><p>Prepare meals for residents.</p></div><script type="application/ld+json">invalid</script>`,
	})
	jobs, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/jobs", Adapter: "hireserve", Confirmed: true})
	if ch.Status != "checked" || len(jobs) != 1 || jobs[0].Posted != "2026-10-29" || jobs[0].Deadline != "2026-11-12" || jobs[0].Salary != "£25,000" || len(jobs[0].Matched) != 0 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
}
func TestWMJobsWhitespacePaginationAndLocationArray(t *testing.T) {
	n, _ := parseHTML(strings.NewReader("<h1>Found 52 jobs</h1><a href=\" \n /job/123/officer/ \n \">Role</a><a href=\"?Page=2\" rel=\"next\">Next</a>"))
	ls := links(n, "https://example.org/searchjobs/?Page=1", "wmjobs")
	if len(ls.jobs) != 1 || ls.next != "https://example.org/searchjobs/?Page=2" || advertisedTotal(n) != 52 {
		t.Fatalf("%+v", ls)
	}
	n, _ = parseHTML(strings.NewReader(`<script type="application/ld+json">{"@type":"JobPosting","title":"Officer","description":"Provide IT support","jobLocation":[{"address":{"addressLocality":"Birmingham"}}]}</script>`))
	jobs := parseJobs(n, "https://example.org/job/123/officer/", "Example Council")
	if len(jobs) != 1 || jobs[0].Location != "Birmingham" {
		t.Fatal(jobs)
	}
}
func TestJobtrainStopsAtLastOffset(t *testing.T) {
	n, _ := parseHTML(strings.NewReader(`<input id="totalMatchRecords" value="24"><input id="totalCurrentRecords" value="12">`))
	u, _ := url.Parse("https://example.org/cards?Skip=12")
	if jobtrainNext(n, u) != "" {
		t.Fatal("must stop at last page")
	}
}
func TestSecurityChallengeReportedWithoutRetry(t *testing.T) {
	count := 0
	s := NewScanner()
	s.Delay = 0
	s.Client = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`<title>MHR Cloud Services | Challenge</title>`)), Header: http.Header{}, Request: r}, nil
	})}
	_, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/jobs", Adapter: "itrent", Confirmed: true})
	if ch.Status != "blocked" || count != 1 {
		t.Fatalf("%+v requests=%d", ch, count)
	}
}

func TestITrentFullDescriptionsAndSessionPrivacy(t *testing.T) {
	s := fixtureScanner(map[string]string{
		"/search?wvid=public": `<form id="ETREC179GF" action="ETREC179GF?USESSION=anonymous-private-session&amp;WVID=public&amp;LANG=USA"></form>`,
		"/etrec106gf.json?LANG=USA&USESSION=anonymous-private-session&WVID=public":                 `{"search":{"total_rec":1,"rec_from":1,"rec_to":1},"results":[{"vacancy_id":"vac1","job_title":"Cook"}]}`,
		"/etrec107gf.json?LANG=USA&USESSION=anonymous-private-session&VACANCY_ID=vac1&WVID=public": `[{"job_title":"Job title","job_titles":"Cook","job_description":"<p>Prepare meals for residents.</p>","vacancy_d":"20261029","app_close_d":"20261112","salary":"£25,000","location":"Example town","copy_link":"https://example.org/ETREC107GF.open?USESSION=anonymous-private-session"}]`,
	})
	jobs, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/search?wvid=public", Adapter: "itrent", Confirmed: true})
	if ch.Status != "checked" || len(jobs) != 1 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
	j := jobs[0]
	if j.Title != "Cook" || j.Posted != "2026-10-29" || j.Deadline != "2026-11-12" || len(j.Matched) != 0 || strings.Contains(j.URL, "SESSION") || strings.Contains(j.URL, "anonymous-private-session") {
		t.Fatalf("unexpected public job: %+v", j)
	}
}
func TestITrentUnexpectedResponseNotEmptySuccess(t *testing.T) {
	s := fixtureScanner(map[string]string{
		"/search": `<form id="ETREC179GF" action="ETREC179GF?USESSION=session&amp;WVID=public"></form>`,
		"/etrec106gf.json?USESSION=session&WVID=public": `{"error":"temporarily unavailable"}`,
	})
	_, ch := s.ScanCouncil(context.Background(), Council{Name: "Example Council", URL: "https://example.org/search", Adapter: "itrent", Confirmed: true})
	if ch.Status != "failed" {
		t.Fatalf("%+v", ch)
	}
}
