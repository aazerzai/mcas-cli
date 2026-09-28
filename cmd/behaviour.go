package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/dental-dash/my-child-at-school-cli/internal/cache"
	"github.com/dental-dash/my-child-at-school-cli/internal/config"
	"github.com/dental-dash/my-child-at-school-cli/internal/mcas"
	"github.com/dental-dash/my-child-at-school-cli/internal/output"
)

// behaviourDayConcurrency caps simultaneous per-day detail fetches. If the
// proxy starts answering 429/5xx under load, lower it rather than retrying.
const behaviourDayConcurrency = 4

var (
	behaviourDate  string
	behaviourLimit int
)

var behaviourCmd = &cobra.Command{
	Use:   "behaviour",
	Short: "Show behaviour points and events for the academic year, or a single day with --date",
	RunE:  runBehaviour,
}

func init() {
	behaviourCmd.Flags().StringVar(&behaviourDate, "date", "", "show behaviour events for a single day, YYYY-MM-DD, instead of the year summary")
	behaviourCmd.Flags().IntVar(&behaviourLimit, "limit", 0, "show at most the newest N events (default: all)")
	rootCmd.AddCommand(behaviourCmd)
}

func runBehaviour(cmd *cobra.Command, args []string) error {
	if behaviourLimit < 0 {
		return fmt.Errorf("invalid --limit %d: must be 0 or more", behaviourLimit)
	}
	dayFilter := ""
	if behaviourDate != "" {
		day, err := parseDateFlag(behaviourDate)
		if err != nil {
			return err
		}
		dayFilter = day.Format(dateFormat)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	format, err := resolveFormat(cfg)
	if err != nil {
		return err
	}
	creds, err := cfg.ResolveCredentials(flagEmail, flagPassword)
	if err != nil {
		return emitFailure(format, "behaviour", output.AuthErrorType, err)
	}
	cacheDir, err := cfg.CacheDir()
	if err != nil {
		return err
	}
	c, err := cache.New(cacheDir, cfg.CacheTTL())
	if err != nil {
		return err
	}

	// One logged-in client, created on first use, so a fully cached run
	// never logs in.
	var (
		once      sync.Once
		client    *mcas.Client
		loginErr  error
		getClient = func() (*mcas.Client, error) {
			once.Do(func() {
				client = mcas.New(creds)
				if loginErr = client.Login(); loginErr == nil {
					_, loginErr = client.LoadYearID()
				}
			})
			return client, loginErr
		}
	)
	keyPrefix := "behaviour:" + creds.Email + ":"

	year, err := fetchCached(c, keyPrefix+"year", forceRefresh, func() (*mcas.BehaviourYear, error) {
		cl, err := getClient()
		if err != nil {
			return nil, err
		}
		return cl.BehaviourYear()
	})
	if err != nil {
		return emitFailure(format, "behaviour", classifyError(err), err)
	}

	selected := selectBehaviourEvents(year.Events, dayFilter, behaviourLimit)
	fetchDay := func(date string) ([]mcas.BehaviourDayEvent, error) {
		return fetchCached(c, keyPrefix+"day:"+date, forceRefresh, func() ([]mcas.BehaviourDayEvent, error) {
			cl, err := getClient()
			if err != nil {
				return nil, err
			}
			day, err := time.Parse(dateFormat, date)
			if err != nil {
				return nil, err
			}
			return cl.BehaviourDay(day)
		})
	}
	stderr := cmd.ErrOrStderr()
	if err := enrichBehaviourEvents(selected, fetchDay, func(msg string) {
		fmt.Fprintln(stderr, "warning: "+msg)
	}); err != nil {
		return emitFailure(format, "behaviour", classifyError(err), err)
	}

	if dayFilter != "" {
		return output.Result(os.Stdout, format, "behaviour", selected, renderBehaviourDayText)
	}
	shown := *year
	shown.Events = selected
	return output.Result(os.Stdout, format, "behaviour", &shown, renderBehaviourYearText)
}

// selectBehaviourEvents keeps the events on day (ISO date; "" for any), then
// the newest limit of them (0 = all). events must already be newest-first.
// The result is a copy, and never nil.
func selectBehaviourEvents(events []mcas.BehaviourEvent, day string, limit int) []mcas.BehaviourEvent {
	selected := make([]mcas.BehaviourEvent, 0, len(events))
	for _, e := range events {
		if day == "" || e.Date.Format(dateFormat) == day {
			selected = append(selected, e)
		}
	}
	if limit > 0 && len(selected) > limit {
		selected = selected[:limit]
	}
	return selected
}

// enrichBehaviourEvents fills in description, teacher, class and outcome for
// events by fetching each distinct event day once, up to
// behaviourDayConcurrency at a time. A day that fails to load, or whose rows
// don't line up with its events, is left as is and reported through warn.
// Only an auth failure is returned.
func enrichBehaviourEvents(events []mcas.BehaviourEvent, fetchDay func(date string) ([]mcas.BehaviourDayEvent, error), warn func(string)) error {
	var days []string
	seen := map[string]bool{}
	for _, e := range events {
		if d := e.Date.Format(dateFormat); !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}

	type result struct {
		rows []mcas.BehaviourDayEvent
		err  error
	}
	results := make([]result, len(days))
	sem := make(chan struct{}, behaviourDayConcurrency)
	var wg sync.WaitGroup
	for i, d := range days {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i].rows, results[i].err = fetchDay(d)
		}()
	}
	wg.Wait()

	for i, d := range days {
		r := results[i]
		if r.err != nil {
			var authErr *mcas.AuthError
			if errors.As(r.err, &authErr) {
				return r.err
			}
			warn(fmt.Sprintf("couldn't load details for %s: %v", d, r.err))
			continue
		}
		if err := mcas.ApplyDayDetails(events, d, r.rows); err != nil {
			warn(fmt.Sprintf("couldn't match details for %s: %v", d, err))
		}
	}
	return nil
}

func renderBehaviourDayText(w io.Writer, data any) error {
	events := data.([]mcas.BehaviourEvent)
	if len(events) == 0 {
		_, err := fmt.Fprintln(w, "No behaviour events recorded.")
		return err
	}
	return writeBehaviourEvents(w, events)
}

func renderBehaviourYearText(w io.Writer, data any) error {
	year := data.(*mcas.BehaviourYear)
	if _, err := fmt.Fprintf(w, "%s: %d points (%d positive, %d negative)\n", year.YearName, year.Points.Total, year.Points.Positive, year.Points.Negative); err != nil {
		return err
	}
	if len(year.Events) == 0 {
		_, err := fmt.Fprintln(w, "No behaviour events recorded.")
		return err
	}
	return writeBehaviourEvents(w, year.Events)
}

func writeBehaviourEvents(w io.Writer, events []mcas.BehaviourEvent) error {
	for _, e := range events {
		if _, err := fmt.Fprintln(w, formatBehaviourEvent(e)); err != nil {
			return err
		}
	}
	return nil
}

// formatBehaviourEvent renders one event as
// "- 2026-09-07 12:57 [Negative -1] 7X · Mr A Teacher · Description (OUTCOME)".
// The subject (else the class) leads; empty segments are left out along with
// their separators.
func formatBehaviourEvent(e mcas.BehaviourEvent) string {
	when := e.Date.Format(dateFormat)
	if e.Date.Hour() != 0 || e.Date.Minute() != 0 {
		when += " " + e.Date.Format("15:04")
	}
	line := fmt.Sprintf("- %s [%s %+d]", when, e.Type, e.Points)

	what := e.Description
	if e.Outcome != "" {
		if what != "" {
			what += " "
		}
		what += "(" + e.Outcome + ")"
	}
	lead := e.Subject
	if lead == "" {
		lead = e.Class
	}
	var parts []string
	for _, p := range []string{lead, e.Teacher, what} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " · ")
	}
	return line
}
