package tracker

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestTribePadPaginationAcrossAllCategories(t *testing.T) {
	s := fixtureScanner(map[string]string{"/jobs/search": `<a href="/jobs/job/Cook/1">Cook</a><a href="/jobs/search/-1/2">Next results page &gt;</a>`, "/jobs/search/-1/2": `<a href="/jobs/job/Officer/2">Officer</a>`, "/jobs/job/Cook/1": `<script type="application/ld+json">{"@type":"JobPosting","title":"Cook","description":"Prepare food"}</script>`, "/jobs/job/Officer/2": `<script type="application/ld+json">{"@type":"JobPosting","title":"Officer","description":"Maintain ICT services"}</script>`})
	js, ch := s.ScanCouncil(context.Background(), Council{Name: "Council", URL: "https://example.org/jobs/search", Adapter: "tribepad", Confirmed: true})
	if ch.Status != "checked" || len(js) != 2 || len(js[0].Matched) != 0 || len(js[1].Matched) == 0 {
		t.Fatalf("%+v %+v", ch, js)
	}
}
func TestJGPDescriptionAndLabelledDeadline(t *testing.T) {
	n, _ := parseHTML(strings.NewReader(`<script type="application/ld+json">{"@type":"JobPosting","title":"Officer","datePosted":"2026-11-01"}</script><article><header class="vacancy/header"><h2>Officer</h2></header><div class="row"><div>Closing Date:</div><div>22/11/2026 at 23:59</div></div><section class="vacancy/details">Maintain ICT systems for residents.</section></article>`))
	js := jgpDetail(n, "https://example.org/vacancies/1", "Council")
	if len(js) != 1 || js[0].Deadline != "2026-11-22" || js[0].Posted != "2026-11-01" || len(js[0].Matched) == 0 {
		t.Fatal(js)
	}
}
func TestBucksMatchesFullAdvertNotTeaser(t *testing.T) {
	n, _ := parseHTML(strings.NewReader(`<main>Full advert includes ICT skills.</main>`))
	js := bucksFullDescription(n, []Job{{Title: "Officer", Description: "Join us"}})
	if len(js) != 1 || len(js[0].Matched) == 0 {
		t.Fatal(js)
	}
}
func TestITrentPaginationReadsLaterPages(t *testing.T) {
	q := url.Values{"WVID": {"view"}, "USESSION": {"anonymous"}, "LANG": {"USA"}}
	endpoint := func(name string, v url.Values) string { return "/" + name + "?" + v.Encode() }
	pages := map[string]string{"/jobs?WVID=view": `<form id="ETREC179GF" action="?WVID=view&amp;USESSION=anonymous&amp;LANG=USA"></form>`}
	pages[endpoint("etrec106gf.json", q)] = `{"results":[{"vacancy_id":"1","job_title":"Cook"}],"search":{"total_rec":"2","rec_from":"1","rec_to":"1","results_pp":1}}`
	next := url.Values{}
	for k, v := range q {
		next[k] = v
	}
	next.Set("RESULTS_PP", "1")
	next.Set("TOTAL_REC", "2")
	next.Set("REC_FROM", "2")
	next.Set("REC_TO", "2")
	pages[endpoint("etrec106gf.json", next)] = `{"results":[{"vacancy_id":"2","job_title":"Officer"}],"search":{"total_rec":2,"rec_from":2,"rec_to":2}}`
	for _, id := range []string{"1", "2"} {
		d := url.Values{}
		for k, v := range q {
			d[k] = v
		}
		d.Set("VACANCY_ID", id)
		pages[endpoint("etrec107gf.json", d)] = `[{"job_description":"Maintain ICT services","vacancy_d":"20261101","app_close_d":"20261122"}]`
	}
	js, ch := fixtureScanner(pages).ScanCouncil(context.Background(), Council{Name: "Council", URL: "https://example.org/jobs?WVID=view", Adapter: "itrent", Confirmed: true})
	if ch.Status != "checked" || len(js) != 2 || js[1].Deadline != "2026-11-22" {
		t.Fatalf("%+v %+v", ch, js)
	}
	delete(pages, endpoint("etrec106gf.json", next))
	js, ch = fixtureScanner(pages).ScanCouncil(context.Background(), Council{Name: "Council", URL: "https://example.org/jobs?WVID=view", Adapter: "itrent", Confirmed: true})
	if ch.Status != "partial" || len(js) != 1 {
		t.Fatalf("missing page: %+v %+v", ch, js)
	}
}
func TestChallengeRecognised(t *testing.T) {
	if !securityChallenge([]byte("We just need to confirm you're a real person.")) {
		t.Fatal("challenge missed")
	}
}
func TestUnexpectedEmptySearchIsNotChecked(t *testing.T) {
	s := fixtureScanner(map[string]string{"/jobs": "jobs page", "/wp-admin/admin-ajax.php": `{"advertsHtml":"<p>changed response</p>"}`})
	_, ch := s.ScanCouncil(context.Background(), Council{Name: "Council", URL: "https://example.org/jobs", Adapter: "cardiff", Confirmed: true})
	if ch.Status == "checked" {
		t.Fatal(ch)
	}
}

func TestCardiffPostingDateUsesPublishedTimezone(t *testing.T) {
	if got := cardiffDate("2026-10-22 23:00:00", "Europe/London"); got != "2026-10-23" {
		t.Fatal(got)
	}
	if got := cardiffDate("2026-11-22 23:59:59", "Europe/London"); got != "2026-11-22" {
		t.Fatal(got)
	}
}

func TestITrentEmptyObjectRequiresExplicitZeroTotal(t *testing.T) {
	for _, total := range []string{"0", "1"} {
		q := url.Values{"WVID": {"view"}, "USESSION": {"anonymous"}}
		pages := map[string]string{"/jobs?WVID=view": `<form id="ETREC179GF" action="?WVID=view&amp;USESSION=anonymous"></form>`, "/etrec106gf.json?" + q.Encode(): `{"results":{},"search":{"total_rec":` + total + `,"rec_to":""}}`}
		js, ch := fixtureScanner(pages).ScanCouncil(context.Background(), Council{Name: "Council", URL: "https://example.org/jobs?WVID=view", Adapter: "itrent", Confirmed: true})
		if total == "0" && (ch.Status != "checked" || len(js) != 0) {
			t.Fatal(ch)
		}
		if total == "1" && ch.Status == "checked" {
			t.Fatal("advertised vacancies silently accepted as zero")
		}
	}
}
