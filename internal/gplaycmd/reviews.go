package gplaycmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kirvigen/droidship/internal/play"
)

// pageSize is how many reviews we ask Play for at a time.
const pageSize = 100

// cmdReviews lists reviews, newest first.
func cmdReviews(ctx context.Context, c *play.Client, args []string) error {
	pkg, fs, err := packageAndFlags(args, "reviews")
	if err != nil {
		return err
	}
	stars := fs.Int("stars", 0, "show only reviews with this star rating")
	unreplied := fs.Bool("unreplied", false, "show only reviews without a developer reply")
	limit := fs.Int("limit", 0, "stop after this many reviews")
	translate := fs.String("translate", "", "ask Play to translate reviews into this language")
	asJSON := fs.Bool("json", false, "print JSON")
	full := fs.Bool("full", false, "print the whole review text instead of one line")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	reviews, err := collectReviews(ctx, c, pkg, *translate, *limit, *stars, *unreplied)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(reviews)
	}
	if len(reviews) == 0 {
		fmt.Println("no reviews matched — note that Play only returns reviews from the last 7 days")
		return nil
	}
	if *full {
		printReviewsFull(reviews)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "STARS\tDATE\tVERSION\tREPLIED\tAUTHOR\tTEXT\tREVIEW ID")
	for _, r := range reviews {
		u := r.User()
		if u == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			starsLabel(u.StarRating), u.LastModified.Time().Format("2006-01-02"),
			dash(u.AppVersionName), repliedLabel(r), dash(oneLine(r.AuthorName, 18)),
			oneLine(u.Text, 60), r.ReviewID)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d review(s). Play only serves the last 7 days through this API.\n", len(reviews))
	return nil
}

// collectReviews pages through the API and applies the client-side filters.
func collectReviews(ctx context.Context, c *play.Client, pkg, translate string, limit, stars int, unreplied bool) ([]play.Review, error) {
	var out []play.Review
	token := ""
	for {
		batch, next, err := c.Reviews(ctx, pkg, play.ReviewsOpts{
			MaxResults:          pageSize,
			Token:               token,
			TranslationLanguage: translate,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range batch {
			if stars > 0 && r.Stars() != stars {
				continue
			}
			if unreplied && r.Reply() != nil {
				continue
			}
			out = append(out, r)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
		if next == "" || len(batch) == 0 {
			return out, nil
		}
		token = next
	}
}

// printReviewsFull prints each review as a readable block.
func printReviewsFull(reviews []play.Review) {
	for i, r := range reviews {
		u := r.User()
		if u == nil {
			continue
		}
		if i > 0 {
			fmt.Println(strings.Repeat("─", 72))
		}
		fmt.Printf("%s  %s  %s  %s\n", starsLabel(u.StarRating),
			u.LastModified.Time().Format("2006-01-02 15:04"),
			dash(u.AppVersionName), dash(u.Device))
		fmt.Printf("author: %s\n", dash(r.AuthorName))
		fmt.Printf("id    : %s\n\n", r.ReviewID)
		fmt.Println(strings.TrimSpace(u.Text))
		if rep := r.Reply(); rep != nil {
			fmt.Printf("\n  ↳ our reply (%s):\n  %s\n",
				rep.LastModified.Time().Format("2006-01-02"), strings.TrimSpace(rep.Text))
		}
		fmt.Println()
	}
}

// cmdReply publishes a developer reply to one review.
func cmdReply(ctx context.Context, c *play.Client, args []string) error {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("usage: droidship gplay reply <package> <reviewId> (--text S | --text-file F)")
	}
	pkg, reviewID := args[0], args[1]

	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	text := fs.String("text", "", "reply text")
	textFile := fs.String("text-file", "", "read the reply from a file")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if (*text == "") == (*textFile == "") {
		return fmt.Errorf("provide exactly one of --text or --text-file")
	}
	body, err := resolveNotes(*text, *textFile)
	if err != nil {
		return err
	}
	if n := len([]rune(body)); n > play.MaxReplyLen {
		return fmt.Errorf("reply is %d characters, Play allows %d — shorten it", n, play.MaxReplyLen)
	}

	reply, err := c.ReplyToReview(ctx, pkg, reviewID, body)
	if err != nil {
		return err
	}
	fmt.Printf("replied to %s (%d/%d chars):\n%s\n", reviewID, len([]rune(body)), play.MaxReplyLen, reply.Text)
	return nil
}

func starsLabel(n int) string {
	if n < 1 || n > 5 {
		return "—"
	}
	return strings.Repeat("★", n) + strings.Repeat("·", 5-n)
}

func repliedLabel(r play.Review) string {
	if rep := r.Reply(); rep != nil {
		return rep.LastModified.Time().Format("2006-01-02")
	}
	return "—"
}

var _ = time.Time{}
