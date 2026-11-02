package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Read the public candidate Atom feed, never Oleeo's internal structured feed.
func (s *Scanner) scanOleeo(ctx context.Context, c Council) ([]Job, Check) {
	ch := Check{Name: c.Name, URL: c.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	fail := func(err error) ([]Job, Check) {
		ch.Status = "failed"
		if strings.Contains(err.Error(), "security challenge") {
			ch.Status = "blocked"
		}
		ch.Message = err.Error()
		return []Job{}, ch
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return fail(err)
	}
	marker := "/candidate/jobboard/vacancy/"
	start := strings.Index(u.Path, marker)
	if start < 0 {
		return fail(fmt.Errorf("public vacancy board ID not found"))
	}
	id := strings.Split(strings.TrimPrefix(u.Path[start:], marker), "/")[0]
	if id == "" {
		return fail(fmt.Errorf("public vacancy board ID missing"))
	}
	u.Path = marker + id + "/feed"
	u.RawQuery = ""
	u.Fragment = ""
	// Respect the provider's ten-second crawl delay, including after robots.txt.
	oldDelay := s.Delay
	if s.Delay < 10*time.Second {
		s.Delay = 10 * time.Second
	}
	defer func() { s.Delay = oldDelay }()
	data, err := s.data(ctx, u.String())
	if err != nil {
		return fail(err)
	}
	jobs, err := parseOleeoFeed(data, c)
	if err != nil {
		return fail(err)
	}
	ch.Status = "partial"
	ch.Message = fmt.Sprintf("Public candidate feed: %d vacancy summaries collected. Keyword matching uses titles and short summaries only; full descriptions and application deadlines are unavailable. Publication dates come from the feed. Feed completeness cannot be independently verified.", len(jobs))
	return jobs, ch
}
func parseOleeoFeed(data []byte, c Council) ([]Job, error) {
	var feed struct {
		XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
		Entries []struct {
			ID        string `xml:"id"`
			Title     string `xml:"title"`
			Published string `xml:"published"`
			Content   struct {
				Inner string `xml:",innerxml"`
			} `xml:"content"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("invalid public Atom feed: %w", err)
	}
	out := []Job{}
	seen := map[string]bool{}
	board, _ := url.Parse(c.URL)
	for _, entry := range feed.Entries {
		u, err := url.Parse(strings.TrimSpace(entry.ID))
		title := strip(entry.Title)
		if err != nil || !safeURL(entry.ID) || u.Host != board.Host || !strings.Contains(u.Path, "/opp/") || title == "" {
			return nil, fmt.Errorf("invalid vacancy in public feed")
		}
		if seen[u.String()] {
			continue
		}
		seen[u.String()] = true
		desc := strip(entry.Content.Inner)
		sum := sha256.Sum256([]byte(c.Name + "|" + u.String()))
		out = append(out, Job{ID: hex.EncodeToString(sum[:12]), Council: c.Name, Employer: c.Name, Title: title, URL: u.String(), Description: desc, Posted: date(entry.Published), Sponsorship: "Not stated — verify with employer", Matched: Matches(title + " " + desc)})
	}
	return out, nil
}
