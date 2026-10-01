package rustorecmd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kirvigen/droidship/internal/rustore"
	"github.com/kirvigen/droidship/internal/store"
)

func TestStoreNotesAndListingAreUnsupported(t *testing.T) {
	s := &Store{}
	if err := s.Notes(context.Background(), "com.x", "ru-RU", "x"); !errors.Is(err, store.ErrUnsupported) {
		t.Fatalf("notes: %v", err)
	}
	if _, err := s.Listing(context.Background(), "com.x", "ru-RU"); !errors.Is(err, store.ErrUnsupported) {
		t.Fatalf("listing: %v", err)
	}
	if s.ReplyLimit() != 500 {
		t.Fatal("RuStore allows 500 characters")
	}
}

func TestPartialPercentMustBeARuStoreStep(t *testing.T) {
	if _, err := partial(33); err == nil {
		t.Fatal("33% is not a RuStore rollout step")
	}
	if _, err := partial(12.5); err == nil {
		t.Fatal("12.5% is not a RuStore rollout step")
	}
	if p, err := partial(25); err != nil || p != 25 {
		t.Fatalf("25%%: %d %v", p, err)
	}
	if p, err := partial(0); err != nil || p != 0 {
		t.Fatal("0 means everyone")
	}
}

func TestJoinReviews(t *testing.T) {
	now := time.Now()
	list := []rustoreComment{
		{id: "7", stars: 2, text: " slow ", when: now},
		{id: "8", stars: 5, when: now},
		{id: "9", stars: 2, when: now.AddDate(0, 0, -30)},
	}
	replies := map[string]string{"7": "поправили"}

	all := joinReviews(list, replies, store.ReviewsQuery{Days: 7})
	if len(all) != 2 || all[0].Text != "slow" || !all[0].Replied || all[0].Reply != "поправили" {
		t.Fatalf("days filter and join: %+v", all)
	}
	if got := joinReviews(list, replies, store.ReviewsQuery{Unanswered: true}); len(got) != 2 || got[0].ID != "8" {
		t.Fatalf("unanswered filter: %+v", got)
	}
	if got := joinReviews(list, replies, store.ReviewsQuery{Stars: 2, Limit: 1}); len(got) != 1 || got[0].ID != "7" {
		t.Fatalf("stars and limit: %+v", got)
	}
}

func TestPlanFromVersions(t *testing.T) {
	versions := []rustore.Version{
		{VersionID: 3, VersionName: "1.4.16", VersionCode: 29, VersionStatus: "MODERATION"},
		{VersionID: 2, VersionName: "1.4.14", VersionCode: 27, VersionStatus: "ACTIVE"},
	}
	p := planFromVersions(versions, store.PublishRequest{AAB: "/b/app.aab"})
	if p.Live != "1.4.14 (27)" || !strings.Contains(p.Action, "app.aab") || !strings.Contains(p.Action, "you release") ||
		!strings.Contains(p.Note, "1.4.16 (29) is in MODERATION") {
		t.Fatalf("%+v", p)
	}
	p = planFromVersions(nil, store.PublishRequest{APK: "app.apk", GoLive: true})
	if p.Live != "nothing live" || !strings.Contains(p.Action, "live after moderation") {
		t.Fatalf("%+v", p)
	}
}

func TestReplyEditsAnExistingAnswer(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if key != "POST /public/auth" {
			calls = append(calls, key)
		}
		switch key {
		case "POST /public/auth":
			fmt.Fprint(w, `{"code":"OK","body":{"jwe":"t","ttl":900}}`)
		case "GET /public/v1/application/com.x/feedback":
			fmt.Fprint(w, `{"code":"OK","body":[{"id":"5","commentId":"7","status":"DELETED"},{"id":"6","commentId":"7","status":"PUBLISHED"}]}`)
		default:
			fmt.Fprint(w, `{"code":"OK","body":{"id":1}}`)
		}
	}))
	defer srv.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	c := rustore.NewWithKey("42", key)
	c.BaseURL = srv.URL
	s := &Store{client: c}

	if err := s.Reply(t.Context(), "com.x", "7", "новый"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reply(t.Context(), "com.x", "8", "первый"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /public/v1/application/com.x/feedback", "POST /public/v1/application/com.x/feedback/6",
		"GET /public/v1/application/com.x/feedback", "POST /public/v1/application/com.x/feedback",
	}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v", calls)
	}
}
