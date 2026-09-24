package cli

import (
	"io"
	"os"
	"strconv"
)

// termWidth is the column count of the terminal w writes to: $COLUMNS when
// set, else the terminal size; 0 when w is not a terminal.
func termWidth(w io.Writer) int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	if f, ok := w.(*os.File); ok {
		return ttyColumns(f)
	}
	return 0
}

// textBudget is how many runes a trailing free-text column may take so a row
// of fixed columns (their widths plus two spaces each) fits the terminal.
// Without a terminal it is def; it never drops below min.
func textBudget(width int, fixed []int, def, min int) int {
	if width <= 0 {
		return def
	}
	used := 0
	for _, f := range fixed {
		used += f + 2
	}
	if n := width - used - 1; n > min {
		return n
	}
	return min
}
