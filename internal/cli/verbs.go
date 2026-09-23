package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kirvigen/droidship/internal/store"
)

// openStore and isConfigured are variables so tests can swap in fakes.
var (
	openStore    = openRealStore
	isConfigured = realConfigured
)

// writeVerbs change something in a store, so they never default to "every store".
var writeVerbs = map[string]bool{"publish": true, "release": true, "rollout": true, "notes": true, "reply": true}

// verbs maps each unified verb to its implementation.
var verbs = map[string]func(v *verbRun) error{
	"auth": verbAuth, "status": verbStatus, "publish": verbPublish, "release": verbRelease,
	"rollout": verbRollout, "notes": verbNotes, "reviews": verbReviews, "reply": verbReply,
	"listing": verbListing,
}

// verbRun carries one unified-verb invocation.
type verbRun struct {
	name       string
	args       []string
	code       int
	fs         *flag.FlagSet
	positional []string
	stores     []string
	json       bool
	stdout     io.Writer
	stderr     io.Writer
	ctx        context.Context
}

// usageError marks a mistake on the command line (exit 2).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func runVerb(name string, args []string, stdout, stderr io.Writer) int {
	v := &verbRun{
		name: name, args: args, fs: flag.NewFlagSet(name, flag.ContinueOnError),
		stdout: stdout, stderr: stderr, ctx: context.Background(),
	}
	v.fs.SetOutput(stderr)
	err := verbs[name](v)
	var ue usageError
	switch {
	case err == nil:
		return v.code
	case errors.Is(err, flag.ErrHelp):
		return ExitUsage
	case errors.As(err, &ue):
		fmt.Fprintln(stderr, "error:", err)
		return ExitUsage
	default:
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
}

// parse splits leading positionals from flags, so both `status com.x --store gplay`
// and `status --store gplay com.x` work, then resolves --store. Each verb
// registers its own flags on v.fs before calling it.
func (v *verbRun) parse() error {
	var storeFlag string
	v.fs.StringVar(&storeFlag, "store", "", "gplay | rustore | appgallery | a comma list | all")
	v.fs.BoolVar(&v.json, "json", false, "print JSON")
	i := 0
	for i < len(v.args) && !strings.HasPrefix(v.args[i], "-") {
		i++
	}
	v.positional = append(v.positional, v.args[:i]...)
	if err := v.fs.Parse(v.args[i:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{err.Error()}
	}
	v.positional = append(v.positional, v.fs.Args()...)

	if storeFlag == "" && writeVerbs[v.name] {
		return usageError{fmt.Sprintf("--store is required for %s (gplay, rustore, appgallery, a comma list, or all)", v.name)}
	}
	if storeFlag == "" || storeFlag == "all" {
		for _, s := range store.All {
			if isConfigured(s) {
				v.stores = append(v.stores, s)
			} else if storeFlag == "all" {
				fmt.Fprintf(v.stderr, "%s: not configured, skipped\n", s)
			}
		}
		if len(v.stores) == 0 {
			return errors.New("no store is configured: see the Credentials section of the README")
		}
		return nil
	}
	for _, s := range strings.Split(storeFlag, ",") {
		s = strings.TrimSpace(s)
		if !contains(store.All, s) {
			return usageError{fmt.Sprintf("unknown store %q: use gplay, rustore or appgallery", s)}
		}
		if !contains(v.stores, s) {
			v.stores = append(v.stores, s)
		}
	}
	return nil
}

// need returns the n-th positional argument or a usage error naming it.
func (v *verbRun) need(n int, what string) (string, error) {
	if len(v.positional) <= n {
		return "", usageError{fmt.Sprintf("%s: %s is required", v.name, what)}
	}
	return v.positional[n], nil
}

// result is one store's outcome of a verb.
type result struct {
	Store string
	Value any
	Err   error
}

// each runs fn for every selected store, in order, and collects the outcomes.
func (v *verbRun) each(fn func(s store.Store) (any, error)) []result {
	var out []result
	for _, name := range v.stores {
		s, err := openStore(name)
		if err != nil {
			out = append(out, result{Store: name, Err: err})
			continue
		}
		val, err := fn(s)
		out = append(out, result{Store: name, Value: val, Err: err})
	}
	return out
}

// exitCode folds the outcomes: any error is 1, else any unsupported is 3.
func exitCode(results []result) int {
	code := ExitOK
	for _, r := range results {
		switch {
		case r.Err == nil:
		case errors.Is(r.Err, store.ErrUnsupported):
			if code == ExitOK {
				code = ExitUnsupported
			}
		default:
			code = ExitError
		}
	}
	return code
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// sortReviews orders reviews newest first.
func sortReviews(r []store.Review) {
	sort.SliceStable(r, func(i, j int) bool { return r[i].Time.After(r[j].Time) })
}

// readText returns inline text or the trimmed contents of a file.
func readText(inline, path, flagName string) (string, error) {
	if inline != "" && path != "" {
		return "", usageError{fmt.Sprintf("use either --%s or --%s-file, not both", flagName, flagName)}
	}
	if path == "" {
		return inline, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}
