package tracker

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func oracleFixtureURL(resource, finder, expand string) string {
	q := url.Values{"onlyData": {"true"}, "finder": {finder}}
	if expand != "" {
		q.Set("expand", expand)
	}
	return oracleAPI + resource + "?" + q.Encode()
}
func oracleFixtureCouncil() Council {
	return Council{Name: "Example Council", URL: "https://example.org/hcmUI/CandidateExperience/en/sites/CX_1", Adapter: "oracle", Confirmed: true}
}
func TestOraclePaginationFullTextAndDates(t *testing.T) {
	pages := map[string]string{
		oracleFixtureURL("recruitingCEJobRequisitions", "findReqs;siteNumber=CX_1,limit=25,offset=0", "requisitionList"): `{"items":[{"SiteNumber":"CX_1","TotalJobsCount":2,"requisitionList":[{"Id":"1","Title":"Cook","PostedDate":"2026-10-28","PostingEndDate":"2026-11-11"}]}]}`,
		oracleFixtureURL("recruitingCEJobRequisitions", "findReqs;siteNumber=CX_1,limit=25,offset=1", "requisitionList"): `{"items":[{"SiteNumber":"CX_1","TotalJobsCount":2,"requisitionList":[{"Id":"2","Title":"Digital Officer"}]}]}`,
		oracleFixtureURL("recruitingCEJobRequisitionDetails", "ById;Id=1,siteNumber=CX_1", ""):                           `{"items":[{"Id":"1","Title":"Cook","ExternalDescriptionStr":"Prepare meals for residents.","PrimaryLocation":"Example Town"}]}`,
		oracleFixtureURL("recruitingCEJobRequisitionDetails", "ById;Id=2,siteNumber=CX_1", ""):                           `{"items":[{"Id":"2","Title":"Digital Officer","ExternalDescriptionStr":"<p>Salary £46,579 - £55,224. Maintain council services.</p>","ExternalQualificationsStr":"ICT experience required.","ExternalResponsibilitiesStr":"Sponsorship is not available for this post.","ExternalPostedStartDate":"2026-10-29T13:56:26+00:00","ExternalPostedEndDate":"2026-11-10T23:55:00+00:00"}]}`,
	}
	jobs, ch := fixtureScanner(pages).ScanCouncil(context.Background(), oracleFixtureCouncil())
	if ch.Status != "checked" || ch.Reviewed != 2 || len(jobs) != 2 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
	if len(jobs[0].Matched) != 0 || jobs[0].Posted != "2026-10-28" {
		t.Fatalf("non-keyword role not retained correctly: %+v", jobs[0])
	}
	j := jobs[1]
	if j.Posted != "2026-10-29" || j.Deadline != "2026-11-10" || j.Salary != "£46,579 - £55,224" || j.Sponsorship != "Advert says no visa sponsorship" || !strings.Contains(j.Description, "ICT experience") || !strings.HasSuffix(j.URL, "/sites/CX_1/job/2") {
		t.Fatalf("unexpected full advert: %+v", j)
	}
}
func TestOracleMissingPaginationIsPartial(t *testing.T) {
	pages := map[string]string{
		oracleFixtureURL("recruitingCEJobRequisitions", "findReqs;siteNumber=CX_1,limit=25,offset=0", "requisitionList"): `{"items":[{"SiteNumber":"CX_1","TotalJobsCount":2,"requisitionList":[{"Id":"1","Title":"Cook"}]}]}`,
		oracleFixtureURL("recruitingCEJobRequisitionDetails", "ById;Id=1,siteNumber=CX_1", ""):                           `{"items":[{"Id":"1","Title":"Cook","ExternalDescriptionStr":"Prepare meals."}]}`,
	}
	jobs, ch := fixtureScanner(pages).ScanCouncil(context.Background(), oracleFixtureCouncil())
	if ch.Status != "partial" || len(jobs) != 1 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
}
func TestOracleMalformedSearchIsNotSuccessfulZero(t *testing.T) {
	for _, body := range []string{`{}`, `{"items":[{"SiteNumber":"OTHER","TotalJobsCount":0,"requisitionList":[]}]}`, `{"items":[{"SiteNumber":"CX_1","requisitionList":[]}]}`} {
		pages := map[string]string{oracleFixtureURL("recruitingCEJobRequisitions", "findReqs;siteNumber=CX_1,limit=25,offset=0", "requisitionList"): body}
		_, ch := fixtureScanner(pages).ScanCouncil(context.Background(), oracleFixtureCouncil())
		if ch.Status != "failed" {
			t.Fatalf("%s: %+v", body, ch)
		}
	}
}
func TestOracleRejectsWrongDetailID(t *testing.T) {
	summary := oracleVacancy{ID: "1", Title: "Cook"}
	detail := oracleVacancy{ID: "2", Title: "Cook", Description: "Prepare meals"}
	u, _ := url.Parse(oracleFixtureCouncil().URL)
	if _, ok := oracleJob(oracleFixtureCouncil(), u, "CX_1", summary, detail); ok {
		t.Fatal("wrong advert accepted")
	}
}
func TestOracleHonoursJobLimit(t *testing.T) {
	pages := map[string]string{
		oracleFixtureURL("recruitingCEJobRequisitions", "findReqs;siteNumber=CX_1,limit=25,offset=0", "requisitionList"): `{"items":[{"SiteNumber":"CX_1","TotalJobsCount":2,"requisitionList":[{"Id":"1","Title":"Cook"},{"Id":"2","Title":"Officer"}]}]}`,
		oracleFixtureURL("recruitingCEJobRequisitionDetails", "ById;Id=1,siteNumber=CX_1", ""):                           `{"items":[{"Id":"1","Title":"Cook","ExternalDescriptionStr":"Prepare meals"}]}`,
	}
	s := fixtureScanner(pages)
	s.MaxJobs = 1
	jobs, ch := s.ScanCouncil(context.Background(), oracleFixtureCouncil())
	if ch.Status != "partial" || len(jobs) != 1 {
		t.Fatalf("%+v %+v", ch, jobs)
	}
}
