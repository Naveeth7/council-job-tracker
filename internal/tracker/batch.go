package tracker

import (
	"context"
	"net/url"
	"strings"
	"sync"
)

type councilResult struct {
	jobs  []Job
	check Check
}

// Check independent recruitment hosts concurrently, but keep each host's
// councils sequential so the normal request spacing is preserved. Results
// enter state in configuration order; workers never mutate persistent state.
func (s *Scanner) scanBatch(ctx context.Context, councils []Council) []councilResult {
	results := make([]councilResult, len(councils))
	groups := [][]int{}
	byHost := map[string]int{}
	for i, c := range councils {
		if c.URL == "" {
			results[i].jobs, results[i].check = s.ScanCouncil(ctx, c)
			continue
		}
		u, _ := url.Parse(c.URL)
		host := "invalid"
		if u != nil {
			host = strings.ToLower(u.Hostname())
		}
		g, ok := byHost[host]
		if !ok {
			g = len(groups)
			byHost[host] = g
			groups = append(groups, nil)
		}
		groups[g] = append(groups[g], i)
	}
	tasks := make(chan []int)
	var wg sync.WaitGroup
	var callback sync.Mutex
	workers := 4
	if len(groups) < workers {
		workers = len(groups)
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range tasks {
				child := NewScanner()
				child.Client = s.Client
				child.Delay = s.Delay
				child.MaxPages = s.MaxPages
				child.MaxJobs = s.MaxJobs
				for _, i := range group {
					results[i].jobs, results[i].check = child.ScanCouncil(ctx, councils[i])
					if s.OnCheck != nil {
						callback.Lock()
						s.OnCheck(results[i].check)
						callback.Unlock()
					}
				}
			}
		}()
	}
	for _, g := range groups {
		tasks <- g
	}
	close(tasks)
	wg.Wait()
	return results
}
