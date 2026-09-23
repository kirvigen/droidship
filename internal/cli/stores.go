package cli

import (
	"fmt"

	"github.com/kirvigen/droidship/internal/appgallerycmd"
	"github.com/kirvigen/droidship/internal/gplaycmd"
	"github.com/kirvigen/droidship/internal/rustorecmd"
	"github.com/kirvigen/droidship/internal/store"
)

// openRealStore builds the adapter for one store from the configured credentials.
func openRealStore(name string) (store.Store, error) {
	switch name {
	case store.GPlay:
		return gplaycmd.NewStore()
	case store.RuStore:
		return rustorecmd.NewStore()
	case store.AppGallery:
		return appgallerycmd.NewStore()
	}
	return nil, fmt.Errorf("unknown store %q", name)
}

// realConfigured reports whether a store has credentials, without contacting it.
func realConfigured(name string) bool {
	switch name {
	case store.GPlay:
		return gplaycmd.Configured()
	case store.RuStore:
		return rustorecmd.Configured()
	case store.AppGallery:
		return appgallerycmd.Configured()
	}
	return false
}
