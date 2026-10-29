package tracker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func text(n *html.Node) string {
	if n == nil {
		return ""
	}
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "nav" || n.Data == "footer") {
		return ""
	}
	s := ""
	if n.Type == html.TextNode {
		s = n.Data
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		s += " " + text(c)
	}
	return strings.Join(strings.Fields(s), " ")
}
func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}
func strip(s string) string {
	n, e := html.Parse(strings.NewReader(s))
	if e != nil {
		return s
	}
	return text(n)
}
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func date(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}
func findJobObjects(v any, out *[]map[string]any) {
	switch x := v.(type) {
	case []any:
		for _, y := range x {
			findJobObjects(y, out)
		}
	case map[string]any:
		t := str(x["@type"])
		if t == "JobPosting" {
			*out = append(*out, x)
			return
		}
		if a, ok := x["@type"].([]any); ok {
			for _, s := range a {
				if s == "JobPosting" {
					*out = append(*out, x)
					return
				}
			}
		}
		for _, y := range x {
			findJobObjects(y, out)
		}
	}
}
func parseJobs(n *html.Node, source, council string) []Job {
	objects := []map[string]any{}
	walk(n, func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data == "script" && strings.Contains(attr(x, "type"), "ld+json") && x.FirstChild != nil {
			var v any
			if json.Unmarshal([]byte(x.FirstChild.Data), &v) == nil {
				findJobObjects(v, &objects)
			}
		}
	})
	out := []Job{}
	for _, v := range objects {
		title := strip(str(v["title"]))
		desc := strip(str(v["description"]))
		if title == "" || desc == "" {
			continue
		}
		u := str(v["url"])
		if u == "" {
			u = source
		}
		base, _ := url.Parse(source)
		ref, e := url.Parse(u)
		if e != nil {
			continue
		}
		u = base.ResolveReference(ref).String()
		if !safeURL(u) {
			continue
		}
		id := u
		if m, ok := v["identifier"].(map[string]any); ok && str(m["value"]) != "" {
			id = council + ":" + str(m["value"])
		}
		sum := sha256.Sum256([]byte(council + "|" + id))
		job := Job{ID: hex.EncodeToString(sum[:12]), Council: council, Title: title, Description: desc, URL: u, Posted: date(str(v["datePosted"])), Deadline: date(str(v["validThrough"])), Sponsorship: "Not stated — verify with employer", Matched: Matches(title + " " + desc)}
		if loc, ok := v["jobLocation"].(map[string]any); ok {
			if a, ok := loc["address"].(map[string]any); ok {
				job.Location = str(a["addressLocality"])
				if job.Location == "" {
					job.Location = str(a["streetAddress"])
				}
			}
		}
		if s, ok := v["baseSalary"].(map[string]any); ok {
			if val, ok := s["value"].(map[string]any); ok {
				low, lok := val["minValue"]
				high, hok := val["maxValue"]
				if lok && hok {
					job.Salary = fmt.Sprintf("%v–%v %s / %s", low, high, str(s["currency"]), str(val["unitText"]))
				} else if val["value"] != nil {
					job.Salary = fmt.Sprintf("%v %s / %s", val["value"], str(s["currency"]), str(val["unitText"]))
				}
			}
		}
		lower := strings.ToLower(desc)
		for _, phrase := range []string{"unable to offer visa sponsorship", "cannot offer visa sponsorship", "do not offer visa sponsorship", "not eligible for sponsorship"} {
			if strings.Contains(lower, phrase) {
				job.Sponsorship = "Advert says no visa sponsorship"
			}
		}
		out = append(out, job)
	}
	if len(out) == 0 {
		fields := map[string]string{}
		walk(n, func(x *html.Node) {
			if id := attr(x, "id"); id != "" {
				fields[id] = text(x)
			}
		})
		title := fields["fldjobdescription_jobtitle"]
		desc := fields["fldjobdescription_description1"]
		if title != "" && desc != "" {
			sum := sha256.Sum256([]byte(council + "|" + source))
			j := Job{ID: hex.EncodeToString(sum[:12]), Council: council, Title: title, URL: source, Description: desc, Salary: fields["fldjobdescription_salaryrange"], Location: fields["fldlocation_joblocation"], Matched: Matches(title + " " + desc), Sponsorship: "Not stated — verify with employer"}
			if j.Location == "" {
				j.Location = fields["fldlocation_location_geographicalareacollection"]
			}
			for id, value := range fields {
				if strings.HasSuffix(id, "_divEndDate") {
					re := regexp.MustCompile(`\b\d{2}/\d{2}/\d{4}\b`)
					if d, e := time.Parse("02/01/2006", re.FindString(value)); e == nil {
						j.Deadline = d.Format("2006-01-02")
					}
				}
			}
			if strings.Contains(strings.ToLower(desc), "unable to offer visa sponsorship") {
				j.Sponsorship = "Advert says no visa sponsorship"
			}
			out = append(out, j)
		}
	}
	return out
}

func advertisedTotal(n *html.Node) int {
	s := text(n)
	for _, pattern := range []string{`(?i)Showing\s+\d+\s*[-–]\s*\d+\s+of\s+(\d+)`, `(?i)Number of results:\s*(\d+)`} {
		re := regexp.MustCompile(pattern)
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			v, _ := strconv.Atoi(m[1])
			return v
		}
	}
	return 0
}

type pageLinks struct {
	jobs []string
	next string
}

func links(n *html.Node, base string, adapter string) pageLinks {
	b, _ := url.Parse(base)
	out := pageLinks{jobs: []string{}}
	seen := map[string]bool{}
	walk(n, func(x *html.Node) {
		if x.Type != html.ElementNode || x.Data != "a" {
			return
		}
		ref, e := url.Parse(attr(x, "href"))
		if e != nil {
			return
		}
		u := b.ResolveReference(ref)
		if u.Host != b.Host || !safeURL(u.String()) {
			return
		}
		u.Fragment = ""
		s := u.String()
		p := strings.ToLower(u.Path)
		isJob := (adapter == "myjobscotland" && strings.Contains(p, "/jobs/") && strings.TrimSuffix(p, "/") != strings.TrimSuffix(b.Path, "/")) || (adapter == "talentsoft" && strings.HasPrefix(p, "/job/job-") && strings.HasSuffix(p, ".aspx"))
		if isJob && !seen[s] {
			out.jobs = append(out.jobs, s)
			seen[s] = true
		}
		label := strings.ToLower(text(x))
		rel := strings.Fields(attr(x, "rel"))
		isNext := false
		for _, r := range rel {
			if r == "next" {
				isNext = true
			}
		}
		if label == "next" || label == "next page" || strings.Contains(strings.ToLower(attr(x, "title")), "next page") {
			isNext = true
		}
		if isNext {
			out.next = s
		}
	})
	return out
}
func parseHTML(r io.Reader) (*html.Node, error) { return html.Parse(r) }
func safeURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
