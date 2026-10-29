package tracker

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var Keywords = []string{"IT", "ICT", "digital", "technical", "analyst", "web"}

// IT is intentionally an uppercase acronym: the ordinary pronoun "it" must not match.
var keywordPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bIT\b`), regexp.MustCompile(`(?i)\bICT\b`),
	regexp.MustCompile(`(?i)\bdigital\b`), regexp.MustCompile(`(?i)\btechnical\b`),
	regexp.MustCompile(`(?i)\banalysts?\b`), regexp.MustCompile(`(?i)\bweb\b`),
}

type Council struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Adapter   string `json:"adapter"`
	Confirmed bool   `json:"confirmed"`
	Notes     string `json:"notes"`
	Routes    string `json:"routes"`
}
type Job struct {
	ID          string   `json:"id"`
	Council     string   `json:"council"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Salary      string   `json:"salary"`
	Posted      string   `json:"posted"`
	Deadline    string   `json:"deadline"`
	Sponsorship string   `json:"sponsorship"`
	Matched     []string `json:"matched"`
	FirstSeen   string   `json:"first_seen"`
	LastSeen    string   `json:"last_seen"`
	Status      string   `json:"status"`
}
type Check struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CheckedAt string `json:"checked_at"`
	Reviewed  int    `json:"reviewed"`
}
type Report struct {
	GeneratedAt string   `json:"generated_at"`
	Keywords    []string `json:"keywords"`
	Jobs        []Job    `json:"jobs"`
	Checks      []Check  `json:"checks"`
}
type State struct {
	Jobs     map[string]Job  `json:"jobs"`
	Notified map[string]bool `json:"notified"`
}

func NewState() State { return State{map[string]Job{}, map[string]bool{}} }
func Matches(s string) []string {
	out := []string{}
	for i, p := range keywordPatterns {
		if p.MatchString(s) {
			out = append(out, Keywords[i])
		}
	}
	return out
}

// Publish a short source excerpt, not a full mirror of each vacancy description.
func Excerpt(s string) string {
	start := -1
	for _, p := range keywordPatterns {
		if loc := p.FindStringIndex(s); loc != nil && (start < 0 || loc[0] < start) {
			start = loc[0]
		}
	}
	if start < 0 {
		start = 0
	}
	runes := []rune(s)
	startRune := len([]rune(s[:start]))
	if startRune > 50 {
		startRune -= 50
	} else {
		startRune = 0
	}
	end := startRune + 480
	if end > len(runes) {
		end = len(runes)
	}
	out := string(runes[startRune:end])
	if startRune > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

func PublicReport(r Report) Report {
	r.Jobs = append([]Job{}, r.Jobs...)
	for i := range r.Jobs {
		r.Jobs[i].Description = Excerpt(r.Jobs[i].Description)
	}
	return r
}
func ReadCouncils(path string) ([]Council, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, e := r.Read()
	if e != nil {
		return nil, e
	}
	fields := map[string]int{}
	for i, h := range header {
		fields[strings.TrimPrefix(h, "\ufeff")] = i
	}
	for _, h := range []string{"name", "url", "adapter", "confirmed", "notes", "routes"} {
		if _, ok := fields[h]; !ok {
			return nil, fmt.Errorf("missing CSV column %s", h)
		}
	}
	rows, e := r.ReadAll()
	if e != nil {
		return nil, e
	}
	out := []Council{}
	seen := map[string]bool{}
	for _, row := range rows {
		v := func(k string) string { return strings.TrimSpace(row[fields[k]]) }
		n := v("name")
		if n == "" || seen[strings.ToLower(n)] {
			return nil, fmt.Errorf("empty or duplicated council name")
		}
		seen[strings.ToLower(n)] = true
		out = append(out, Council{n, v("url"), v("adapter"), strings.EqualFold(v("confirmed"), "true"), v("notes"), v("routes")})
	}
	return out, nil
}
func WriteJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func IsClosed(deadline string, now time.Time) bool {
	if len(deadline) < 10 {
		return false
	}
	d, e := time.Parse("2006-01-02", deadline[:10])
	if e != nil {
		return false
	}
	return now.UTC().Format("2006-01-02") > d.Format("2006-01-02")
}
