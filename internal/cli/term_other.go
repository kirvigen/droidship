//go:build !darwin && !linux

package cli

import "os"

// ttyColumns is not implemented here; tables fall back to fixed widths.
func ttyColumns(*os.File) int { return 0 }
