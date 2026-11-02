package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Buckinghamshire's JSON-LD contains only a short teaser. Search the full
// visible advert rather than treating that teaser as the job description.
func bucksFullDescription(n *html.Node, jobs []Job) []Job {
	var full string
	walk(n, func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data == "main" {
			full = text(x)
		}
	})
	if full == "" {
		return nil
	}
	for i := range jobs {
		jobs[i].Description = full
		jobs[i].Matched = Matches(jobs[i].Title + " " + full)
	}
	return jobs
}

func hasClass(n *html.Node, class string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// publicPortalLinks reads the same anonymous search endpoints used by the
// recruitment pages. It never invokes application, account, or login actions.
func (s *Scanner) publicPortalLinks(ctx context.Context, c Council) ([]string, map[string]Job, int, error) {
	base, _ := url.Parse(c.URL)
	if _, err := s.page(ctx, c.URL); err != nil {
		return nil, nil, 0, err
	}
	urls := []string{}
	seen := map[string]bool{}
	metadata := map[string]Job{}
	total := 0
	observed := 0
	for page := 1; page <= s.MaxPages; page++ {
		var raw string
		var form url.Values
		if c.Adapter == "cardiff" {
			raw = base.ResolveReference(&url.URL{Path: "/wp-admin/admin-ajax.php"}).String()
			form = url.Values{"action": {"lumesse_ajax_grid_list"}, "lanugage": {"1"}, "params[0][key]": {"per_page"}, "params[0][val]": {"50"}, "params[1][key]": {"page_job"}, "params[1][val]": {strconv.Itoa(page)}, "params[2][key]": {"sort"}, "params[2][val]": {"recent"}, "lang": {"en_GB"}}
		} else {
			raw = base.ResolveReference(&url.URL{Path: "/V2/Vacancy/ApplySearchFilter"}).String()
			form = url.Values{"searchControlViewModel.PageNo": {strconv.Itoa(page)}, "searchControlViewModel.Criteria": {""}, "searchControlViewModel.Type": {""}}
		}
		data, err := s.requestData(ctx, raw, form, nil)
		if err != nil {
			return urls, metadata, total, err
		}
		var response struct {
			HTML    string `json:"advertsHtml"`
			Count   *int   `json:"advertsCount"`
			Results string `json:"searchResults"`
		}
		if json.Unmarshal(data, &response) != nil {
			return urls, metadata, total, fmt.Errorf("Public search returned unreadable data")
		}
		content := response.HTML
		if c.Adapter == "engageats" {
			content = response.Results
		}
		if content == "" {
			return urls, metadata, total, fmt.Errorf("Public search returned no result markup")
		}
		n, _ := parseHTML(strings.NewReader(content))
		found := 0
		if c.Adapter == "cardiff" {
			if response.Count == nil {
				return urls, metadata, total, fmt.Errorf("Public search did not advertise a vacancy total")
			}
			total = *response.Count
			walk(n, func(x *html.Node) {
				if x.Data != "a" {
					return
				}
				u := resolvePublicURL(base, attr(x, "href"))
				if u != "" && strings.HasPrefix(mustPath(u), "/jobs/") && !seen[u] {
					seen[u] = true
					urls = append(urls, u)
					found++
				}
			})
		} else {
			re := regexp.MustCompile(`(?i)(\d+) Job openings`)
			if m := re.FindStringSubmatch(text(n)); len(m) > 1 {
				total, _ = strconv.Atoi(m[1])
			} else {
				return urls, metadata, total, fmt.Errorf("Public search did not advertise a vacancy total")
			}
			walk(n, func(card *html.Node) {
				if !hasClass(card, "ats-gray-panel") && !hasClass(card, "ats-white-panel") {
					return
				}
				var raw string
				var cannock bool
				walk(card, func(x *html.Node) {
					if hasClass(x, "btn-search-results-view") {
						raw = resolvePublicURL(base, attr(x, "data-param1"))
					}
					if x.Data == "img" && strings.Contains(strings.ToLower(attr(x, "src")), "cannock") {
						cannock = true
					}
				})
				if raw == "" || seen[raw] {
					return
				}
				seen[raw] = true
				found++
				// This portal serves two councils. Only the Cannock-branded cards belong here.
				if !cannock {
					return
				}
				urls = append(urls, raw)
				t := text(card)
				j := Job{}
				for label, target := range map[string]*string{"Posted:": &j.Posted, "Closing Date:": &j.Deadline} {
					re := regexp.MustCompile(regexp.QuoteMeta(label) + `\s*(\d{2}/\d{2}/\d{4})`)
					if m := re.FindStringSubmatch(t); len(m) > 1 {
						*target = vacancyDate(m[1])
					}
				}
				metadata[raw] = j
			})
		}
		observed += found
		if observed >= total {
			if c.Adapter == "engageats" {
				total = len(urls)
			}
			return urls, metadata, total, nil
		}
		if found == 0 || len(urls) >= s.MaxJobs {
			break
		}
	}
	return urls, metadata, total, fmt.Errorf("Public search pagination was incomplete")
}
func mustPath(raw string) string { u, _ := url.Parse(raw); return u.Path }
func resolvePublicURL(base *url.URL, raw string) string {
	r, e := url.Parse(strings.TrimSpace(raw))
	if e != nil {
		return ""
	}
	u := base.ResolveReference(r)
	if u.Host != base.Host || !safeURL(u.String()) {
		return ""
	}
	u.Fragment = ""
	return u.String()
}
func publicPortalDetail(n *html.Node, raw string, c Council, meta Job) []Job {
	title, desc := "", ""
	walk(n, func(x *html.Node) {
		if c.Adapter == "cardiff" {
			if x.Data == "h1" {
				title = text(x)
			}
			if x.Data == "main" {
				desc = text(x)
			}
		} else {
			if hasClass(x, "ats-title-font") && attr(x, "role") == "heading" {
				title = text(x)
			}
			if hasClass(x, "ats-normal-font") && hasClass(x, "pb-3") && len(text(x)) > len(desc) {
				desc = text(x)
			}
		}
	})
	if title == "" || desc == "" {
		return nil
	}
	if c.Adapter == "cardiff" {
		walk(n, func(x *html.Node) {
			if x.Data != "script" || x.FirstChild == nil {
				return
			}
			for label, target := range map[string]*string{"posting_start_date": &meta.Posted, "posting_end_date": &meta.Deadline} {
				re := regexp.MustCompile(`writePostingDate\("postingDate_` + label + `",\s*"(\d{4}-\d{2}-\d{2})`)
				if m := re.FindStringSubmatch(x.FirstChild.Data); len(m) > 1 {
					*target = m[1]
				}
			}
		})
	}
	sum := sha256.Sum256([]byte(c.Name + "|" + raw))
	j := Job{ID: hex.EncodeToString(sum[:12]), Council: c.Name, Employer: c.Name, Title: title, Description: desc, URL: raw, Posted: meta.Posted, Deadline: meta.Deadline, Sponsorship: "Not stated — verify with employer", Matched: Matches(title + " " + desc)}
	return []Job{j}
}

// JGP's JobPosting block may omit description and deadline. Read those fields
// from the labelled advert instead of silently dropping a valid vacancy.
func jgpDetail(n *html.Node, raw, council string) []Job {
	fields := map[string]string{}
	title, desc, posted := "", "", ""
	walk(n, func(x *html.Node) {
		if hasClass(x, "vacancy/header") {
			walk(x, func(h *html.Node) {
				if h.Data == "h2" {
					title = text(h)
				}
			})
		}
		if hasClass(x, "vacancy/details") {
			desc = text(x)
		}
		if hasClass(x, "row") {
			var cells []string
			for y := x.FirstChild; y != nil; y = y.NextSibling {
				if y.Type == html.ElementNode {
					cells = append(cells, text(y))
				}
			}
			if len(cells) == 2 {
				fields[cells[0]] = cells[1]
			}
		}
		if x.Data == "script" && strings.Contains(attr(x, "type"), "ld+json") && x.FirstChild != nil {
			var v any
			if json.Unmarshal([]byte(x.FirstChild.Data), &v) == nil {
				objects := []map[string]any{}
				findJobObjects(v, &objects)
				if len(objects) > 0 {
					posted = vacancyDate(str(objects[0]["datePosted"]))
				}
			}
		}
	})
	if title == "" || desc == "" {
		return nil
	}
	deadline := regexp.MustCompile(`\d{2}/\d{2}/\d{4}`).FindString(fields["Closing Date:"])
	sum := sha256.Sum256([]byte(council + "|" + raw))
	return []Job{{ID: hex.EncodeToString(sum[:12]), Council: council, Employer: fields["Employer:"], Title: title, Description: desc, URL: raw, Posted: posted, Deadline: vacancyDate(deadline), Salary: fields["Salary:"], Location: fields["Location:"], Matched: Matches(title + " " + desc), Sponsorship: "Not stated — verify with employer"}}
}

func tribePadDetail(n *html.Node, raw, council string) []Job {
	title, desc := "", ""
	fields := map[string]string{}
	walk(n, func(x *html.Node) {
		if x.Data == "h1" {
			title = text(x)
		}
		if attr(x, "id") == "job_main" {
			desc = text(x)
		}
		if hasClass(x, "job-details-overview") {
			walk(x, func(li *html.Node) {
				if li.Data != "li" {
					return
				}
				label := ""
				walk(li, func(a *html.Node) {
					if hasClass(a, "label") {
						label = text(a)
					}
				})
				if label != "" {
					fields[label] = strings.TrimSpace(strings.TrimPrefix(text(li), label))
				}
			})
		}
	})
	if title == "" || desc == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(council + "|" + raw))
	return []Job{{ID: hex.EncodeToString(sum[:12]), Council: council, Title: title, Description: desc, URL: raw, Posted: vacancyDate(fields["Posted on"]), Deadline: vacancyDate(fields["Closing Date"]), Salary: fields["Salary"], Location: fields["Location"], Matched: Matches(title + " " + desc), Sponsorship: "Not stated — verify with employer"}}
}
