package cli

import (
	"fmt"

	"github.com/kirvigen/droidship/internal/gplaycmd"
	"github.com/kirvigen/droidship/internal/store"
)

// openRealStore builds the adapter for one store from the configured credentials.
func openRealStore(name string) (store.Store, error) {
	switch name {
	case store.GPlay:
		return gplaycmd.NewStore()
	}
	return nil, fmt.Errorf("%s: adapter not wired yet", name)
}

// realConfigured reports whether a store has credentials, without contacting it.
func realConfigured(name string) bool {
	switch name {
	case store.GPlay:
		return gplaycmd.Configured()
	}
	return false
}
