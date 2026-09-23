package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kirvigen/droidship/internal/store"
)

// report prints the outcomes of a verb and records the exit code. Errors go to
// stderr; values go to stdout as a table built by rows, or as one JSON array
// in which every object carries "store".
func (v *verbRun) report(results []result, header string, rows func(r result) []string) {
	v.code = exitCode(results)
	v.printErrors(results)
	if v.json {
		var list []any
		for _, r := range results {
			if r.Err != nil {
				list = append(list, map[string]any{"store": r.Store, "error": r.Err.Error(),
					"unsupported": errors.Is(r.Err, store.ErrUnsupported)})
				continue
			}
			list = append(list, withStore(r.Store, r.Value)...)
		}
		v.emitJSON(list)
		return
	}
	if v.code == ExitOK || anySucceeded(results) {
		v.printTable(results, header, rows)
	}
}

func anySucceeded(results []result) bool {
	for _, r := range results {
		if r.Err == nil {
			return true
		}
	}
	return false
}

func (v *verbRun) printTable(results []result, header string, rows func(r result) []string) {
	w := tabwriter.NewWriter(v.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, header)
	for _, r := range results {
		if r.Err == nil {
			for _, line := range rows(r) {
				fmt.Fprintln(w, line)
			}
		}
	}
	_ = w.Flush()
}

// printErrors writes one line per failed store to stderr.
func (v *verbRun) printErrors(results []result) {
	for _, r := range results {
		if r.Err == nil {
			continue
		}
		msg := r.Err.Error()
		if !errors.Is(r.Err, store.ErrUnsupported) {
			msg = r.Store + ": " + msg
		}
		fmt.Fprintln(v.stderr, msg)
	}
}

func (v *verbRun) emitJSON(value any) {
	if value == nil {
		value = []any{}
	}
	enc := json.NewEncoder(v.stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(value)
}

// withStore flattens a store's value into JSON objects that carry "store".
func withStore(name string, value any) []any {
	raw, _ := json.Marshal(value)
	var many []map[string]any
	if json.Unmarshal(raw, &many) == nil {
		out := make([]any, 0, len(many))
		for _, m := range many {
			m["store"] = name
			out = append(out, m)
		}
		return out
	}
	one := map[string]any{}
	_ = json.Unmarshal(raw, &one)
	one["store"] = name
	return []any{one}
}

// oneLine flattens text to a single line no longer than max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func verbAuth(v *verbRun) error {
	if err := v.parse(); err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) { return s.Auth(v.ctx) })
	v.report(results, "STORE\tIDENTITY\tCREDENTIALS FROM", func(r result) []string {
		a := r.Value.(store.AuthInfo)
		return []string{r.Store + "\t" + a.Identity + "\t" + a.Source}
	})
	return nil
}

func verbStatus(v *verbRun) error {
	if err := v.parse(); err != nil {
		return err
	}
	pkg, err := v.need(0, "<package>")
	if err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) { return s.Status(v.ctx, pkg) })
	v.report(results, "STORE\tTRACK\tVERSION\tSTATUS\tROLLOUT\tID", func(r result) []string {
		var lines []string
		for _, x := range r.Value.([]store.Release) {
			pct := ""
			if x.Percent > 0 {
				pct = fmt.Sprintf("%g%%", x.Percent)
			}
			lines = append(lines, strings.Join([]string{r.Store, x.Track, x.Version, x.Status, pct, x.ID}, "\t"))
		}
		return lines
	})
	return nil
}

func verbPublish(v *verbRun) error {
	req := store.PublishRequest{}
	var notes, notesFile string
	v.fs.StringVar(&req.AAB, "aab", "", "Android App Bundle to upload")
	v.fs.StringVar(&req.APK, "apk", "", "APK to upload (RuStore, AppGallery)")
	v.fs.StringVar(&notes, "notes", "", "release notes")
	v.fs.StringVar(&notes, "whats-new", "", "alias of --notes")
	v.fs.StringVar(&notesFile, "notes-file", "", "read release notes from a file")
	v.fs.StringVar(&notesFile, "whats-new-file", "", "alias of --notes-file")
	v.fs.StringVar(&req.Lang, "lang", "ru-RU", "language of the release notes")
	v.fs.Float64Var(&req.Percent, "percent", 0, "staged rollout percent, 0 < p < 100")
	v.fs.BoolVar(&req.GoLive, "go-live", false, "release to users now instead of staging")
	if err := v.parse(); err != nil {
		return err
	}
	pkg, err := v.need(0, "<package>")
	if err != nil {
		return err
	}
	if (req.AAB == "") == (req.APK == "") {
		return usageError{"publish: pass exactly one of --aab or --apk"}
	}
	if req.Percent < 0 || req.Percent >= 100 {
		return usageError{"--percent must be above 0 and below 100 (omit it for everyone)"}
	}
	req.Package = pkg
	if req.Notes, err = readText(notes, notesFile, "notes"); err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) { return s.Publish(v.ctx, req, v.stderr) })
	v.report(results, "STORE\tSTATE\tNEXT", func(r result) []string {
		p := r.Value.(store.PublishResult)
		return []string{r.Store + "\t" + p.State + "\t" + p.Next}
	})
	return nil
}

func verbRelease(v *verbRun) error {
	req := store.ReleaseRequest{}
	v.fs.StringVar(&req.Version, "version", "", "version to release (default: the staged one)")
	v.fs.Float64Var(&req.Percent, "percent", 0, "release to this percent of users first")
	if err := v.parse(); err != nil {
		return err
	}
	var err error
	if req.Package, err = v.need(0, "<package>"); err != nil {
		return err
	}
	if req.Percent < 0 || req.Percent > 100 {
		return usageError{"release: --percent must be between 0 and 100"}
	}
	results := v.each(func(s store.Store) (any, error) {
		return map[string]string{"state": "released"}, s.Release(v.ctx, req, v.stderr)
	})
	v.report(results, "STORE\tSTATE", func(r result) []string { return []string{r.Store + "\treleased"} })
	return nil
}

func verbRollout(v *verbRun) error {
	req := store.RolloutRequest{}
	v.fs.StringVar(&req.Version, "version", "", "version whose rollout to change (default: the live one)")
	v.fs.Float64Var(&req.Percent, "percent", 0, "new rollout percent; 100 completes the rollout")
	if err := v.parse(); err != nil {
		return err
	}
	var err error
	if req.Package, err = v.need(0, "<package>"); err != nil {
		return err
	}
	if req.Percent <= 0 || req.Percent > 100 {
		return usageError{"rollout: --percent between 0 and 100 is required"}
	}
	results := v.each(func(s store.Store) (any, error) {
		return map[string]any{"percent": req.Percent}, s.Rollout(v.ctx, req, v.stderr)
	})
	v.report(results, "STORE\tROLLOUT", func(r result) []string {
		return []string{fmt.Sprintf("%s\t%g%%", r.Store, req.Percent)}
	})
	return nil
}

func verbNotes(v *verbRun) error {
	var text, file, lang string
	v.fs.StringVar(&text, "text", "", "release notes")
	v.fs.StringVar(&file, "text-file", "", "read release notes from a file")
	v.fs.StringVar(&lang, "lang", "ru-RU", "language")
	if err := v.parse(); err != nil {
		return err
	}
	pkg, err := v.need(0, "<package>")
	if err != nil {
		return err
	}
	if text, err = readText(text, file, "text"); err != nil {
		return err
	}
	if text == "" {
		return usageError{"notes: --text or --text-file is required"}
	}
	results := v.each(func(s store.Store) (any, error) {
		return map[string]string{"lang": lang, "state": "updated"}, s.Notes(v.ctx, pkg, lang, text)
	})
	v.report(results, "STORE\tLANG\tSTATE", func(r result) []string {
		return []string{r.Store + "\t" + lang + "\tupdated"}
	})
	return nil
}

func verbReviews(v *verbRun) error {
	q := store.ReviewsQuery{}
	v.fs.IntVar(&q.Stars, "stars", 0, "only this rating, 1..5")
	v.fs.BoolVar(&q.Unanswered, "unanswered", false, "only reviews without a reply")
	v.fs.IntVar(&q.Days, "days", 7, "how many days back")
	v.fs.IntVar(&q.Limit, "limit", 50, "at most this many per store")
	if err := v.parse(); err != nil {
		return err
	}
	var err error
	if q.Package, err = v.need(0, "<package>"); err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) {
		list, err := s.Reviews(v.ctx, q)
		sortReviews(list)
		return list, err
	})
	if v.json {
		// One flat, newest-first array across stores.
		v.code = exitCode(results)
		v.printErrors(results)
		all := []store.Review{}
		for _, r := range results {
			if r.Err == nil {
				all = append(all, r.Value.([]store.Review)...)
			}
		}
		sortReviews(all)
		v.emitJSON(all)
		return nil
	}
	v.report(results, "STORE\tID\tSTARS\tDATE\tREPLIED\tTEXT", func(r result) []string {
		var lines []string
		for _, x := range r.Value.([]store.Review) {
			replied := ""
			if x.Replied {
				replied = "yes"
			}
			lines = append(lines, fmt.Sprintf("%s\t%s\t%d\t%s\t%s\t%s", r.Store, x.ID, x.Stars,
				x.Time.Format(time.DateOnly), replied, oneLine(x.Text, 70)))
		}
		return lines
	})
	return nil
}

func verbReply(v *verbRun) error {
	var text, file string
	v.fs.StringVar(&text, "text", "", "reply text")
	v.fs.StringVar(&file, "text-file", "", "read the reply from a file")
	if err := v.parse(); err != nil {
		return err
	}
	if len(v.stores) != 1 {
		return usageError{"reply: pass exactly one --store; a review id belongs to one store"}
	}
	pkg, err := v.need(0, "<package>")
	if err != nil {
		return err
	}
	id, err := v.need(1, "<reviewId>")
	if err != nil {
		return err
	}
	if text, err = readText(text, file, "text"); err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) {
		if err := store.CheckReply(s.ReplyLimit(), text); err != nil {
			return nil, err
		}
		return map[string]string{"review": id, "state": "replied"}, s.Reply(v.ctx, pkg, id, text)
	})
	v.report(results, "STORE\tREVIEW\tSTATE", func(r result) []string {
		return []string{r.Store + "\t" + id + "\treplied"}
	})
	return nil
}

func verbListing(v *verbRun) error {
	var lang string
	v.fs.StringVar(&lang, "lang", "ru-RU", "language")
	if err := v.parse(); err != nil {
		return err
	}
	pkg, err := v.need(0, "<package>")
	if err != nil {
		return err
	}
	results := v.each(func(s store.Store) (any, error) { return s.Listing(v.ctx, pkg, lang) })
	v.report(results, "STORE\tLANG\tTITLE\tSHORT", func(r result) []string {
		l := r.Value.(store.Listing)
		return []string{r.Store + "\t" + l.Lang + "\t" + l.Title + "\t" + oneLine(l.Short, 60)}
	})
	return nil
}
