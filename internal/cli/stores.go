package cli

import (
	"fmt"

	"github.com/kirvigen/droidship/internal/store"
)

func openRealStore(name string) (store.Store, error) {
	return nil, fmt.Errorf("%s: adapter not wired yet", name)
}

func realConfigured(name string) bool { return false }
