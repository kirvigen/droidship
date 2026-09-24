package cli

import (
	"strings"
	"testing"

	"github.com/kirvigen/droidship/internal/store"
)

func TestTextBudget(t *testing.T) {
	if got := textBudget(0, []int{5, 5}, 70, 20); got != 70 {
		t.Fatalf("no terminal: %d, want the default", got)
	}
	if got := textBudget(100, []int{8, 10}, 70, 20); got != 100-(10+12)-1 {
		t.Fatalf("got %d", got)
	}
	if got := textBudget(30, []int{8, 10}, 70, 20); got != 20 {
		t.Fatalf("narrow terminal: %d, want the minimum", got)
	}
}

func TestReviewsTableFitsCOLUMNS(t *testing.T) {
	t.Setenv("COLUMNS", "80")
	long := strings.Repeat("очень длинный отзыв ", 10)
	withFakes(t, &fake{name: "rustore", reviews: []store.Review{{Store: "rustore", ID: "2418294808", Stars: 5, Text: long}}})
	_, out, _ := run(t, "reviews", "com.x")
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if n := len([]rune(line)); n > 80 {
			t.Fatalf("line of %d runes exceeds COLUMNS=80: %q", n, line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("the text should be cut with an ellipsis:\n%s", out)
	}
}
