package tracker

import "testing"

func TestOleeoFeedDatesAndValidation(t *testing.T) {
	c := Council{Name: "Council", URL: "https://ccc.tal.net/candidate/jobboard/vacancy/3/adv/"}
	raw := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>https://ccc.tal.net/opp/42/en-GB</id><title>Digital Analyst</title><published>2026-11-02T08:00:00Z</published><updated>2026-11-03T08:00:00Z</updated><content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">Contract End Date:2028-03-31</div></content></entry></feed>`
	jobs, e := parseOleeoFeed([]byte(raw), c)
	if e != nil || len(jobs) != 1 {
		t.Fatalf("%v %v", jobs, e)
	}
	if jobs[0].Posted != "2026-11-02" || jobs[0].Deadline != "" || len(jobs[0].Matched) != 2 {
		t.Fatalf("unexpected metadata: %+v", jobs[0])
	}
	for _, bad := range []string{`<html>Quick Check Needed</html>`, `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>https://other.org/opp/1</id><title>Job</title></entry></feed>`} {
		if _, e := parseOleeoFeed([]byte(bad), c); e == nil {
			t.Fatal("accepted invalid feed")
		}
	}
}
