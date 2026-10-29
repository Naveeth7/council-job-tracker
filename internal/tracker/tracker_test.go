package tracker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatches(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int
	}{
		{"It is vital to support communities with ability", 0},
		{"IT Officer provides ICT support and digital web services", 4},
		{"Technical assistant and data analyst", 2},
		{"Capital accounts officer", 0},
	} {
		if got := Matches(tc.input); len(got) != tc.want {
			t.Errorf("%q: got %v", tc.input, got)
		}
	}
}
func TestStructuredJobsAndEscaping(t *testing.T) {
	s := `<script type="application/ld+json">{"@graph":[{"@type":"JobPosting","title":"Digital Officer","description":"<p>Maintain ICT applications.</p>","datePosted":"2026-10-28T00:00:00Z","validThrough":"2026-11-10T23:59:00Z","url":"/vacancy/12"}]}</script>`
	n, e := parseHTML(strings.NewReader(s))
	if e != nil {
		t.Fatal(e)
	}
	j := parseJobs(n, "https://example.org/jobs/12", "Example Council")
	if len(j) != 1 || j[0].Deadline != "2026-11-10" || j[0].URL != "https://example.org/vacancy/12" || len(j[0].Matched) != 2 {
		t.Fatalf("unexpected: %+v", j)
	}
}
func TestPaginationAndDedup(t *testing.T) {
	n, _ := parseHTML(strings.NewReader(`<a href="/councils/a/jobs/digital-1">Job</a><a href="/councils/a/jobs/digital-1">Apply</a><a rel="next" href="?organisation=1&page=1">Next</a><a href="https://other.test/jobs/2">Other</a>`))
	l := links(n, "https://example.org/search-jobs?organisation=1", "myjobscotland")
	if len(l.jobs) != 1 || !strings.Contains(l.next, "page=1") {
		t.Fatalf("unexpected links: %+v", l)
	}
}
func TestDeadlineNotFirstSeen(t *testing.T) {
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, time.UTC)
	if IsClosed("2026-10-28", now) || IsClosed("", now) || !IsClosed("2026-10-27", now) {
		t.Fatal("date-only deadline handling is incorrect")
	}
}
func TestFailureRetainsHistory(t *testing.T) {
	state := NewState()
	state.Jobs["one"] = Job{ID: "one", Council: "A Council", Title: "IT Officer", Status: "listed"}
	c := Council{Name: "A Council", URL: "https://example.org", Adapter: "manual", Confirmed: true}
	r := NewScanner().Run(context.Background(), []Council{c}, &state, time.Now())
	if len(r.Jobs) != 1 || r.Jobs[0].Status != "unverified" || r.Checks[0].Status != "manual_check" {
		t.Fatalf("history should be retained: %+v", r)
	}
}
func TestImportPreservesLinksAndFlagsUncertainCandidates(t *testing.T) {
	d := t.TempDir()
	config := filepath.Join(d, "c.csv")
	register := filepath.Join(d, "r.csv")
	out := filepath.Join(d, "out.csv")
	os.WriteFile(config, []byte("name,url,adapter,confirmed,notes,routes\nA Council,https://a.test,manual,true,,Skilled Worker\nGone Council,,manual,false,,Skilled Worker\n"), 0600)
	os.WriteFile(register, []byte("Organisation Name,Route\nA Council,Skilled Worker\nA Council,Other route\nParochial Church Council,Skilled Worker\n"), 0600)
	if e := ImportReview(register, config, out); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(out)
	s := string(b)
	if !strings.Contains(s, "https://a.test") || !strings.Contains(s, "not_found_review") || !strings.Contains(s, "review_addition") {
		t.Fatal(s)
	}
	c, e := ReadCouncils(out)
	if e != nil || len(c) != 3 {
		t.Fatal(e, c)
	}
	if c[2].Confirmed {
		t.Fatal("new candidates must never be enabled automatically")
	}
}
func TestLocalStateRoundTrip(t *testing.T) {
	t.Setenv("TIDB_HOST", "")
	s, e := OpenStore(context.Background(), filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	state := NewState()
	state.Notified["one"] = true
	if e = s.Save(context.Background(), state); e != nil {
		t.Fatal(e)
	}
	got, e := s.Load(context.Background())
	if e != nil || !got.Notified["one"] {
		t.Fatal(e)
	}
	var b strings.Builder
	json.NewEncoder(&b).Encode(got)
}

func TestTalentsoftFallback(t *testing.T) {
	n, _ := parseHTML(strings.NewReader(`<p id="fldjobdescription_jobtitle">Systems Administrator</p><div id="fldjobdescription_description1">Provide technical finance support. Unable to offer visa sponsorship.</div><p id="fldjobdescription_salaryrange">43000–46000</p><div id="prefix_divEndDate"><h3>Closing Date</h3>06/11/2026</div>`))
	jobs := parseJobs(n, "https://example.org/job/job-systems_1.aspx", "Example Council")
	if len(jobs) != 1 || jobs[0].Deadline != "2026-11-06" || jobs[0].Posted != "" || jobs[0].Sponsorship != "Advert says no visa sponsorship" {
		t.Fatalf("unexpected fallback: %+v", jobs)
	}
}
func TestAdvertisedCount(t *testing.T) {
	for _, s := range []string{"Showing 1 - 25 of 49 available jobs", "Number of results: 49 vacancy(s)"} {
		n, _ := parseHTML(strings.NewReader(s))
		if advertisedTotal(n) != 49 {
			t.Fatal("could not detect pagination coverage")
		}
	}
}

func TestPublicExcerptDoesNotModifyPrivateState(t *testing.T) {
	s := strings.Repeat("Boilerplate statement. ", 80) + "ICT support " + strings.Repeat("more detail ", 80)
	r := Report{Jobs: []Job{{Description: s}}}
	public := PublicReport(r)
	if len([]rune(public.Jobs[0].Description)) > 482 || !strings.Contains(public.Jobs[0].Description, "ICT") || r.Jobs[0].Description != s {
		t.Fatal("public report should have a short matching excerpt and retain original state")
	}
}
