package gplaycmd

import (
	"path/filepath"
	"testing"

	"github.com/kirvigen/droidship/internal/play"
)

func TestToAchievementIncremental(t *testing.T) {
	s := AchievementSpec{
		ID:          "sverka_40",
		Name:        map[string]string{"ru-RU": "Гроссбух закрыт", "en-US": "Ledger closed"},
		Description: map[string]string{"ru-RU": "Сверить все 40 листов.", "en-US": "Check all 40 sheets."},
		Points:      60,
		Steps:       40,
	}
	a, err := s.toAchievement("ru-RU", 12)
	if err != nil {
		t.Fatal(err)
	}
	if a.AchievementType != play.AchievementIncremental || a.StepsToUnlock != 40 {
		t.Fatalf("steps make it incremental: %+v", a)
	}
	if got := a.Draft.Name.Translations[0].Locale; got != "ru-RU" {
		t.Fatalf("primary locale must come first, got %q", got)
	}
	if a.Draft.SortRank != 12 || a.Draft.PointValue != 60 {
		t.Fatalf("rank and points: %+v", a.Draft)
	}
}

func TestToAchievementRejectsBadPoints(t *testing.T) {
	s := AchievementSpec{ID: "x", Name: map[string]string{"ru-RU": "И"}, Description: map[string]string{"ru-RU": "О"}}
	for _, points := range []int64{0, -5, 7} {
		s.Points = points
		if _, err := s.toAchievement("ru-RU", 1); err == nil {
			t.Fatalf("points %d must be rejected: Google takes positive multiples of five", points)
		}
	}
}

func TestToAchievementNeedsPrimaryLocale(t *testing.T) {
	s := AchievementSpec{
		ID:          "x",
		Name:        map[string]string{"en-US": "Name"},
		Description: map[string]string{"ru-RU": "Описание"},
		Points:      5,
	}
	if _, err := s.toAchievement("ru-RU", 1); err == nil {
		t.Fatal("a missing primary-locale name must be an error, not a silent English-only achievement")
	}
}

func TestAchievementChanged(t *testing.T) {
	spec := AchievementSpec{
		ID:          "first_shift",
		Name:        map[string]string{"ru-RU": "Первая смена"},
		Description: map[string]string{"ru-RU": "Отработать первый забег."},
		Points:      5,
	}
	want, err := spec.toAchievement("ru-RU", 1)
	if err != nil {
		t.Fatal(err)
	}
	have := want
	have.ID, have.Token = "CgkI", "AGxgbh"
	if achievementChanged(have, want) {
		t.Fatal("ids assigned by Google are not a difference")
	}
	louder := have
	detail := *have.Draft
	detail.PointValue = 10
	louder.Draft = &detail
	if !achievementChanged(louder, want) {
		t.Fatal("a changed point value must be noticed")
	}
}

func TestCheckImmutable(t *testing.T) {
	std := play.Achievement{AchievementType: play.AchievementStandard}
	inc := play.Achievement{AchievementType: play.AchievementIncremental, StepsToUnlock: 10}
	if err := checkImmutable(std, inc); err == nil {
		t.Fatal("standard cannot become incremental")
	}
	more := inc
	more.StepsToUnlock = 20
	if err := checkImmutable(inc, more); err == nil {
		t.Fatal("stepsToUnlock is fixed at creation")
	}
	if err := checkImmutable(inc, inc); err != nil {
		t.Fatal(err)
	}
}

func TestLockRoundTrip(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "achievements.json")
	path := lockFileFor(spec)
	if want := filepath.Join(dir, "achievements.lock.json"); path != want {
		t.Fatalf("lock file sits next to the spec: got %s, want %s", path, want)
	}
	empty, err := loadLock(path)
	if err != nil || len(empty) != 0 {
		t.Fatalf("a missing lock file is an empty one, not an error: %v %v", empty, err)
	}
	if err := saveLock(path, map[string]string{"first_shift": "CgkI"}); err != nil {
		t.Fatal(err)
	}
	back, err := loadLock(path)
	if err != nil || back["first_shift"] != "CgkI" {
		t.Fatalf("lock round trip: %v %v", back, err)
	}
}

func TestIndexAchievementsAdoptsByName(t *testing.T) {
	made := play.Achievement{
		ID:    "CgkI_manual",
		Draft: &play.AchievementDetail{Name: play.LocalizedStringBundle{Translations: []play.LocalizedString{{Locale: "ru-RU", Value: "Первая смена"}}}},
	}
	locked := play.Achievement{
		ID:    "CgkI_locked",
		Draft: &play.AchievementDetail{Name: play.LocalizedStringBundle{Translations: []play.LocalizedString{{Locale: "ru-RU", Value: "Переезд"}}}},
	}
	index, byName := indexAchievements([]play.Achievement{made, locked}, map[string]string{"relocation_first": "CgkI_locked"}, "ru-RU")
	if index["relocation_first"].ID != "CgkI_locked" {
		t.Fatal("the lock file maps spec ids onto Google ids")
	}
	if byName["Первая смена"].ID != "CgkI_manual" {
		t.Fatal("an achievement made by hand in the console must be adoptable by name")
	}
}
