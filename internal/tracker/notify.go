package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func Notify(ctx context.Context, state *State, report Report) error {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return nil
	}
	from, to := os.Getenv("EMAIL_FROM"), os.Getenv("EMAIL_TO")
	if from == "" || to == "" {
		return errors.New("email is enabled but EMAIL_FROM or EMAIL_TO is missing")
	}
	fresh := []Job{}
	for _, j := range report.Jobs {
		if j.Status == "listed" && !state.Notified[j.ID] {
			fresh = append(fresh, j)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	var body strings.Builder
	fmt.Fprintf(&body, "%d newly discovered council vacancies\nKeywords: IT, ICT, digital, technical, analyst, web\n\nFirst run includes existing vacancies. Missing dates are not inferred.\n\n", len(fresh))
	for _, j := range fresh {
		fmt.Fprintf(&body, "%s — %s\nSalary: %s\nPosted: %s | Deadline: %s\nMatched: %s\nSponsorship: %s\n%s\n\n", j.Title, j.Council, orUnknown(j.Salary), orUnknown(j.Posted), orUnknown(j.Deadline), strings.Join(j.Matched, ", "), j.Sponsorship, j.URL)
	}
	for _, c := range report.Checks {
		if c.Status != "checked" && c.Status != "not_configured" {
			fmt.Fprintf(&body, "Check needs attention: %s — %s\n", c.Name, c.Message)
		}
	}
	b, _ := json.Marshal(map[string]any{"from": from, "to": []string{to}, "subject": fmt.Sprintf("Council jobs: %d newly discovered matches", len(fresh)), "text": body.String()})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 30 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return errors.New("email send failed; new jobs remain pending for a later run")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New("email provider rejected digest; check sender verification and recipient settings")
	}
	for _, j := range fresh {
		state.Notified[j.ID] = true
	}
	return nil
}
func orUnknown(s string) string {
	if s == "" {
		return "Not provided"
	}
	return s
}
