package main

import (
	"context"
	"council-job-tracker/internal/tracker"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) > 1 && os.Args[1] == "import-register" {
		f := flag.NewFlagSet("import-register", flag.ContinueOnError)
		input := f.String("file", "", "New sponsor-register CSV")
		config := f.String("config", "config/councils.csv", "Existing council CSV")
		out := f.String("out", "import-review.csv", "Review CSV (does not replace config)")
		if e := f.Parse(os.Args[2:]); e != nil {
			return e
		}
		if *input == "" {
			return fmt.Errorf("provide -file")
		}
		return tracker.ImportReview(*input, *config, *out)
	}
	f := flag.NewFlagSet("check", flag.ContinueOnError)
	config := f.String("config", "config/councils.csv", "Council CSV")
	out := f.String("out", "site/jobs.json", "Public report")
	statePath := f.String("state", ".state/state.json", "Local state if TiDB is not configured")
	limit := f.Int("limit", 0, "Check only the first N configured councils; for local testing")
	if e := f.Parse(os.Args[1:]); e != nil {
		return e
	}
	councils, e := tracker.ReadCouncils(*config)
	if e != nil {
		return e
	}
	if *limit > 0 {
		selected := []tracker.Council{}
		for _, c := range councils {
			if c.URL != "" {
				selected = append(selected, c)
				if len(selected) >= *limit {
					break
				}
			}
		}
		councils = selected
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()
	if e = os.MkdirAll(filepath.Dir(*statePath), 0700); e != nil {
		return e
	}
	store, e := tracker.OpenStore(ctx, *statePath)
	if e != nil {
		return e
	}
	defer store.Close()
	state, e := store.Load(ctx)
	if e != nil {
		return e
	}
	report := tracker.NewScanner().Run(ctx, councils, &state, time.Now())
	notifyErr := tracker.Notify(ctx, &state, report)
	if e = store.Save(ctx, state); e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(*out), 0755); e != nil {
		return e
	}
	if e = tracker.WriteJSON(*out, tracker.PublicReport(report)); e != nil {
		return e
	}
	for _, c := range report.Checks {
		if c.URL != "" {
			fmt.Printf("%s: %s (%d descriptions reviewed)\n", c.Name, c.Status, c.Reviewed)
		}
	}
	if notifyErr != nil {
		fmt.Fprintln(os.Stderr, "Notification warning:", notifyErr)
	}
	return nil
}
