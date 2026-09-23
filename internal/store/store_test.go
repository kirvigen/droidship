package store

import (
	"errors"
	"strings"
	"testing"
)

func TestUnsupportedIsDetectable(t *testing.T) {
	err := Unsupported("rustore", "notes", "RuStore takes release notes only with a new version")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal("errors.Is(err, ErrUnsupported) = false")
	}
	want := "notes: unsupported by rustore (RuStore takes release notes only with a new version)"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestCheckReply(t *testing.T) {
	if err := CheckReply(350, "ok"); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("я", 351)
	if err := CheckReply(350, long); err == nil {
		t.Fatal("351 runes should exceed a 350 limit")
	}
	if err := CheckReply(0, long); err != nil {
		t.Fatal("0 means the store enforces its own limit")
	}
	if err := CheckReply(350, ""); err == nil {
		t.Fatal("an empty reply is an error")
	}
}
