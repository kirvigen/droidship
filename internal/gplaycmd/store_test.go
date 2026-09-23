package gplaycmd

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirvigen/droidship/internal/play"
	"github.com/kirvigen/droidship/internal/store"
)

func testStore(t *testing.T, f *fakePlay) *Store {
	t.Helper()
	return &Store{client: testClient(t, f)}
}

func TestStoreStatusMapsTracks(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production", Releases: []play.TrackRelease{
		{VersionCodes: []string{"14"}, Status: play.StatusInProgress, UserFraction: 0.2},
		{VersionCodes: []string{"13"}, Status: play.StatusCompleted},
	}}}}
	rows, err := testStore(t, f).Status(context.Background(), "com.x")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Version != "14" || rows[0].Percent != 20 || rows[1].Status != "completed" {
		t.Fatalf("rows = %+v", rows)
	}
	if len(f.deletedEdits) != 1 {
		t.Fatal("a read must throw its edit away")
	}
}

func TestStorePublishStagesADraft(t *testing.T) {
	f := &fakePlay{t: t}
	aab := writeTempFile(t, "app.aab", "AAB")
	res, err := testStore(t, f).Publish(context.Background(),
		store.PublishRequest{Package: "com.x", AAB: aab, Notes: "fixes", Lang: "ru-RU", Percent: 10}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	r := f.trackReceived.Releases[0]
	if f.trackReceived.Track != "production" || r.Status != play.StatusDraft || r.UserFraction != 0 {
		t.Fatalf("track = %+v", f.trackReceived)
	}
	if !strings.Contains(res.Next, "droidship release com.x --store gplay") {
		t.Fatalf("next = %q", res.Next)
	}
}

func TestStorePublishGoLiveWithPercent(t *testing.T) {
	f := &fakePlay{t: t}
	aab := writeTempFile(t, "app.aab", "AAB")
	if _, err := testStore(t, f).Publish(context.Background(),
		store.PublishRequest{Package: "com.x", AAB: aab, GoLive: true, Percent: 20}, io.Discard); err != nil {
		t.Fatal(err)
	}
	r := f.trackReceived.Releases[0]
	if r.Status != play.StatusInProgress || r.UserFraction != 0.2 {
		t.Fatalf("release = %+v", r)
	}
}

func TestStorePublishRejectsAPK(t *testing.T) {
	_, err := (&Store{}).Publish(context.Background(), store.PublishRequest{Package: "com.x", APK: "a.apk"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--aab") {
		t.Fatalf("Play takes bundles only, got %v", err)
	}
}

func TestStoreReleasePicksTheDraft(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production", Releases: []play.TrackRelease{
		{VersionCodes: []string{"15"}, Status: play.StatusDraft},
		{VersionCodes: []string{"14"}, Status: play.StatusCompleted},
	}}}}
	if err := testStore(t, f).Release(context.Background(), store.ReleaseRequest{Package: "com.x"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	r := f.trackReceived.Releases[0]
	if r.VersionCodes[0] != "15" || r.Status != play.StatusCompleted {
		t.Fatalf("release = %+v", r)
	}
}

func TestStoreReleaseWithoutADraft(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production"}}}
	err := testStore(t, f).Release(context.Background(), store.ReleaseRequest{Package: "com.x"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("got %v", err)
	}
}

func TestStoreRolloutWidensTheLiveRelease(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production", Releases: []play.TrackRelease{
		{VersionCodes: []string{"16"}, Status: play.StatusDraft},
		{VersionCodes: []string{"15"}, Status: play.StatusInProgress, UserFraction: 0.1},
	}}}}
	if err := testStore(t, f).Rollout(context.Background(), store.RolloutRequest{Package: "com.x", Percent: 50}, io.Discard); err != nil {
		t.Fatal(err)
	}
	r := f.trackReceived.Releases[0]
	if r.VersionCodes[0] != "15" || r.Status != play.StatusInProgress || r.UserFraction != 0.5 {
		t.Fatalf("release = %+v", r)
	}
}

func TestStoreRolloutToEveryoneCompletes(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production", Releases: []play.TrackRelease{
		{VersionCodes: []string{"15"}, Status: play.StatusInProgress, UserFraction: 0.5},
	}}}}
	if err := testStore(t, f).Rollout(context.Background(), store.RolloutRequest{Package: "com.x", Percent: 100}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if r := f.trackReceived.Releases[0]; r.Status != play.StatusCompleted || r.UserFraction != 0 {
		t.Fatalf("release = %+v", r)
	}
}

func TestStoreNotesReplaceOneLanguage(t *testing.T) {
	f := &fakePlay{t: t, tracks: []play.Track{{Track: "production", Releases: []play.TrackRelease{{
		VersionCodes: []string{"15"}, Status: play.StatusCompleted,
		ReleaseNotes: []play.ReleaseNote{{Language: "en-US", Text: "old en"}, {Language: "ru-RU", Text: "старое"}},
	}}}}}
	if err := testStore(t, f).Notes(context.Background(), "com.x", "ru-RU", "новое"); err != nil {
		t.Fatal(err)
	}
	notes := f.trackReceived.Releases[0].ReleaseNotes
	if len(notes) != 2 || notes[0].Text != "old en" || notes[1].Text != "новое" || f.commits != 1 {
		t.Fatalf("notes = %+v, commits %d", notes, f.commits)
	}
}

func TestStoreReviews(t *testing.T) {
	now := time.Now().Unix()
	f := &fakePlay{t: t, reviews: `{"reviews":[
		{"reviewId":"a","authorName":"Ann","comments":[{"userComment":{"text":" slow ","starRating":2,"lastModified":{"seconds":"` +
		itoa(now) + `"},"appVersionName":"1.4"}},{"developerComment":{"text":"fixed"}}]},
		{"reviewId":"b","authorName":"Bob","comments":[{"userComment":{"text":"old","starRating":5,"lastModified":{"seconds":"` +
		itoa(now-30*86400) + `"}}}]}]}`}
	got, err := testStore(t, f).Reviews(context.Background(), store.ReviewsQuery{Package: "com.x", Days: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" || got[0].Text != "slow" || !got[0].Replied || got[0].Stars != 2 {
		t.Fatalf("reviews = %+v", got)
	}
}

func TestStoreReplyAndLimit(t *testing.T) {
	f := &fakePlay{t: t}
	s := testStore(t, f)
	if s.ReplyLimit() != 350 {
		t.Fatal("Play caps replies at 350")
	}
	if err := s.Reply(context.Background(), "com.x", "a", "спасибо"); err != nil || f.replied != "спасибо" {
		t.Fatalf("reply %q, %v", f.replied, err)
	}
}

func TestStoreListing(t *testing.T) {
	f := &fakePlay{t: t, listing: `{"language":"ru-RU","title":"ГдеБЕНЗ","shortDescription":"Цены","fullDescription":"…"}`}
	l, err := testStore(t, f).Listing(context.Background(), "com.x", "ru-RU")
	if err != nil || l.Title != "ГдеБЕНЗ" || l.Short != "Цены" {
		t.Fatalf("%+v %v", l, err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
