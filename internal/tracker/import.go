package tracker

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ImportReview never overwrites the confirmed council list. Sponsor names alone
// cannot establish local-authority identity; additions and removals require review.
func ImportReview(register, config, out string) error {
	councils, e := ReadCouncils(config)
	if e != nil {
		return e
	}
	known := map[string]Council{}
	for _, c := range councils {
		known[NormalizeName(c.Name)] = c
	}
	f, e := os.Open(register)
	if e != nil {
		return e
	}
	defer f.Close()
	r := csv.NewReader(f)
	h, e := r.Read()
	if e != nil {
		return e
	}
	cols := map[string]int{}
	for i, k := range h {
		cols[strings.TrimPrefix(k, "\ufeff")] = i
	}
	for _, k := range []string{"Organisation Name", "Route"} {
		if _, ok := cols[k]; !ok {
			return fmt.Errorf("register missing %s", k)
		}
	}
	rows, e := r.ReadAll()
	if e != nil {
		return e
	}
	candidates := map[string][]string{}
	present := map[string]bool{}
	routes := map[string]map[string]bool{}
	pattern := regexp.MustCompile(`(?i)\bcouncil\b|\bborough\b|\blocal authority\b|^city and county of |^city of london corporation$`)
	for _, row := range rows {
		n := strings.TrimSpace(row[cols["Organisation Name"]])
		key := NormalizeName(n)
		present[key] = true
		if routes[key] == nil {
			routes[key] = map[string]bool{}
		}
		routes[key][strings.TrimSpace(row[cols["Route"]])] = true
		if pattern.MatchString(n) {
			if _, ok := candidates[key]; !ok {
				candidates[key] = []string{n, "", "manual", "false", "Candidate only: verify this is a local authority before adding.", "", "review_addition"}
			}
		}
	}
	for key, c := range known {
		status := "present"
		note := c.Notes
		if !present[key] {
			status = "not_found_review"
			note += " Not found under this exact normalized name; check renames before removing."
		}
		candidates[key] = []string{c.Name, c.URL, c.Adapter, fmt.Sprint(c.Confirmed), strings.TrimSpace(note), c.Routes, status}
	}
	data := [][]string{}
	for key, row := range candidates {
		if rr := routes[key]; len(rr) > 0 {
			v := []string{}
			for route := range rr {
				v = append(v, route)
			}
			sort.Strings(v)
			row[5] = strings.Join(v, "; ")
		}
		data = append(data, row)
	}
	sort.Slice(data, func(i, j int) bool { return strings.ToLower(data[i][0]) < strings.ToLower(data[j][0]) })
	dest, e := os.Create(out)
	if e != nil {
		return e
	}
	defer dest.Close()
	w := csv.NewWriter(dest)
	if e = w.Write([]string{"name", "url", "adapter", "confirmed", "notes", "routes", "import_status"}); e != nil {
		return e
	}
	w.WriteAll(data)
	return w.Error()
}
