package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/net/html"
	"net/url"
	"strconv"
	"strings"
)

// Follow the card endpoint and offsets advertised by Jobtrain's own page.
func jobtrainNext(n *html.Node, base *url.URL) string {
	endpoint := ""
	total, count := -1, -1
	walk(n, func(x *html.Node) {
		switch attr(x, "id") {
		case "requestUrl":
			endpoint = attr(x, "data-request-url")
		case "totalMatchRecords":
			total, _ = strconv.Atoi(attr(x, "value"))
		case "totalCurrentRecords":
			count, _ = strconv.Atoi(attr(x, "value"))
		}
	})
	if endpoint != "" {
		ref, e := url.Parse(endpoint)
		if e != nil {
			return ""
		}
		next := base.ResolveReference(ref)
		if next.Host != base.Host || !safeURL(next.String()) {
			return ""
		}
		q := next.Query()
		q.Set("Skip", "0")
		next.RawQuery = q.Encode()
		return next.String()
	}
	skip, _ := strconv.Atoi(base.Query().Get("Skip"))
	if count > 0 && total > skip+count {
		next := *base
		q := next.Query()
		q.Set("Skip", strconv.Itoa(skip+count))
		next.RawQuery = q.Encode()
		return next.String()
	}
	return ""
}

type hireserveFeed struct {
	Jobs []struct {
		Title       string `json:"title"`
		Weblink     string `json:"weblink"`
		Publication struct {
			Internet struct {
				PublishDate string `json:"publish_date"`
				ClosingDate string `json:"closing_date"`
			} `json:"internet"`
		} `json:"publication"`
		Classifications map[string]struct {
			Name   string `json:"name"`
			Values []struct {
				Value string `json:"class_val"`
			} `json:"values"`
		} `json:"classifications"`
	} `json:"jobs"`
	Total int `json:"total"`
}

// Read the feed action and hidden parameters from the actual search form.
func (s *Scanner) hireserveLinks(ctx context.Context, c Council) ([]string, map[string]Job, int, error) {
	n, e := s.page(ctx, c.URL)
	if e != nil {
		return nil, nil, 0, e
	}
	b, _ := url.Parse(c.URL)
	var endpoint string
	q := url.Values{}
	walk(n, func(x *html.Node) {
		if x.Data != "form" || !strings.Contains(attr(x, "action"), "ic_job_feeds.feed_engine") {
			return
		}
		endpoint = attr(x, "action")
		walk(x, func(input *html.Node) {
			if input.Data == "input" && attr(input, "type") == "hidden" && attr(input, "name") != "" {
				q.Set(attr(input, "name"), attr(input, "value"))
			}
		})
	})
	if endpoint == "" {
		return nil, nil, 0, errors.New("published vacancy feed was not found")
	}
	ref, e := url.Parse(endpoint)
	if e != nil {
		return nil, nil, 0, errors.New("invalid vacancy feed URL")
	}
	feedURL := b.ResolveReference(ref)
	if feedURL.Host != b.Host {
		return nil, nil, 0, errors.New("vacancy feed changed host; needs verification")
	}
	q.Set("p_summary", "Y")
	feedURL.RawQuery = q.Encode()
	data, e := s.data(ctx, feedURL.String())
	if e != nil {
		return nil, nil, 0, e
	}
	var feed hireserveFeed
	if json.Unmarshal(data, &feed) != nil {
		return nil, nil, 0, errors.New("published vacancy feed could not be parsed")
	}
	urls := []string{}
	meta := map[string]Job{}
	for _, item := range feed.Jobs {
		u, e := url.Parse(item.Weblink)
		if e != nil {
			continue
		}
		u = b.ResolveReference(u)
		if u.Host != b.Host || !safeURL(u.String()) {
			continue
		}
		raw := u.String()
		if _, ok := meta[raw]; ok {
			continue
		}
		sum := sha256.Sum256([]byte(c.Name + "|" + raw))
		j := Job{ID: hex.EncodeToString(sum[:12]), Council: c.Name, Employer: c.Name, Title: strip(item.Title), URL: raw, Posted: date(item.Publication.Internet.PublishDate), Deadline: date(item.Publication.Internet.ClosingDate), Sponsorship: "Not stated — verify with employer"}
		low, high := "", ""
		for _, cl := range item.Classifications {
			vals := []string{}
			for _, v := range cl.Values {
				vals = append(vals, strip(v.Value))
			}
			value := strings.Join(vals, ", ")
			switch cl.Name {
			case "Salary Min":
				low = value
			case "Salary Max":
				high = value
			case "Location":
				j.Location = value
			}
		}
		j.Salary = low
		if high != "" {
			if low != "" {
				j.Salary += "–"
			}
			j.Salary += high
		}
		meta[raw] = j
		urls = append(urls, raw)
	}
	return urls, meta, feed.Total, nil
}

// Some detail pages have invalid JSON-LD; extract the actual advert container.
func hireserveDetail(n *html.Node, j Job) []Job {
	walk(n, func(x *html.Node) {
		for _, cl := range strings.Fields(attr(x, "class")) {
			if cl == "job_description" {
				j.Description = text(x)
			}
		}
	})
	if j.Title == "" || j.Description == "" {
		return nil
	}
	j.Matched = Matches(j.Title + " " + j.Description)
	return []Job{j}
}
