package gplaycmd

// `droidship gplay achievements` drives Play Games Services achievements from a spec
// file, so the whole set of a game is one reviewable JSON in the game's own
// repository instead of a browser session.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/kirvigen/droidship/internal/play"
)

// AchievementSpec is one achievement as the spec file writes it.
type AchievementSpec struct {
	ID          string            `json:"id"` // our own id; Google assigns its own, the lock file keeps the mapping
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Points      int64             `json:"points"`
	Steps       int64             `json:"steps,omitempty"` // >0 makes the achievement incremental
	Hidden      bool              `json:"hidden,omitempty"`
	// AfterEarned is Game Center's second description, shown once the
	// achievement is earned. Play Games has no such field; it lives here so
	// one spec file feeds both stores.
	AfterEarned map[string]string `json:"afterEarned,omitempty"`
	Icon        string            `json:"icon,omitempty"` // 512×512 PNG, relative to the spec file
}

// AchievementsSpec is the whole file: one game project and its achievements.
type AchievementsSpec struct {
	Application   string            `json:"application"` // numeric game project id
	DefaultLocale string            `json:"defaultLocale"`
	Achievements  []AchievementSpec `json:"achievements"`
}

func cmdAchievements(ctx context.Context, c *play.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("droidship gplay achievements (list|sync|delete|publish) — see gplay help")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return cmdAchievementsList(ctx, c, rest)
	case "sync":
		return cmdAchievementsSync(ctx, c, rest, os.Stdout)
	case "delete":
		return cmdAchievementsDelete(ctx, c, rest)
	case "publish":
		return cmdAchievementsPublish(ctx, c, rest)
	default:
		return fmt.Errorf("unknown subcommand %q: droidship gplay achievements (list|sync|delete|publish)", sub)
	}
}

func cmdAchievementsList(ctx context.Context, c *play.Client, args []string) error {
	fs := newFlagSet("achievements list")
	app := fs.String("app", "", "game project id")
	locale := fs.String("locale", "ru", "locale to print")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *app == "" {
		return fmt.Errorf("--app is required: the numeric game project id from Play Console → Play Games Services → Configuration")
	}
	list, err := c.Achievements(ctx, *app)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(list)
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPOINTS\tSTEPS\tSTATE\tPUBLISHED\tNAME")
	for _, a := range list {
		var points int64
		name := ""
		if a.Draft != nil {
			points, name = a.Draft.PointValue, a.Draft.Name.Get(*locale)
		}
		published := "draft only"
		if a.Published != nil {
			published = "live"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
			a.ID, points, play.FormatSteps(a), strings.ToLower(a.InitialState), published, name)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d achievements, %d points of %d\n", len(list), play.TotalPoints(list), play.PointsLimit)
	return nil
}

// cmdAchievementsSync makes the game project match the spec file: it creates
// what is missing and rewrites the draft of what changed. Achievements absent
// from the spec are reported and left alone unless --prune is given, because
// deleting one destroys players' progress on it.
func cmdAchievementsSync(ctx context.Context, c *play.Client, args []string, out io.Writer) error {
	fs := newFlagSet("achievements sync")
	file := fs.String("file", "", "spec file (JSON)")
	app := fs.String("app", "", "game project id (overrides the spec)")
	dry := fs.Bool("dry-run", false, "print the plan, change nothing")
	prune := fs.Bool("prune", false, "delete achievements missing from the spec")
	icons := fs.Bool("icons", true, "upload icons named in the spec for achievements that have none")
	forceIcons := fs.Bool("force-icons", false, "re-upload icons even when the achievement already has one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("--file is required: the achievements spec (JSON)")
	}
	spec, err := loadAchievementsSpec(*file)
	if err != nil {
		return err
	}
	if *app != "" {
		spec.Application = *app
	}
	if spec.Application == "" {
		return fmt.Errorf("no game project id: put \"application\" in the spec or pass --app")
	}

	existing, err := c.Achievements(ctx, spec.Application)
	if err != nil {
		return err
	}
	lockPath := lockFileFor(*file)
	lock, err := loadLock(lockPath)
	if err != nil {
		return err
	}
	index, byName := indexAchievements(existing, lock, spec.DefaultLocale)

	var total int64
	for _, s := range spec.Achievements {
		total += s.Points
	}
	if total > play.PointsLimit {
		return fmt.Errorf("the spec is worth %d points; Google and Apple both cap a game at %d", total, play.PointsLimit)
	}

	specDir := filepath.Dir(*file)
	seen := make(map[string]bool, len(spec.Achievements))
	created, updated, unchanged := 0, 0, 0

	for i, s := range spec.Achievements {
		if s.ID == "" {
			return fmt.Errorf("achievement #%d has no id", i+1)
		}
		if seen[s.ID] {
			return fmt.Errorf("achievement %q appears twice in the spec", s.ID)
		}
		seen[s.ID] = true

		want, err := s.toAchievement(spec.DefaultLocale, int64(i+1))
		if err != nil {
			return fmt.Errorf("%s: %w", s.ID, err)
		}
		have, ok := index[s.ID]
		if !ok {
			have, ok = byName[want.Draft.Name.Get(spec.DefaultLocale)]
		}
		switch {
		case !ok:
			fmt.Fprintf(out, "create  %-20s %3d pts  %s\n", s.ID, s.Points, want.Draft.Name.Get(spec.DefaultLocale))
			if *dry {
				created++
				continue
			}
			made, err := c.CreateAchievement(ctx, spec.Application, want)
			if err != nil {
				return fmt.Errorf("create %s: %w", s.ID, err)
			}
			lock[s.ID] = made.ID
			created++
			if *icons && s.Icon != "" {
				if err := uploadIcon(ctx, c, made.ID, specDir, s.Icon, out); err != nil {
					return err
				}
			}
		case achievementChanged(have, want):
			if err := checkImmutable(have, want); err != nil {
				return fmt.Errorf("%s: %w", s.ID, err)
			}
			fmt.Fprintf(out, "update  %-20s %3d pts  %s\n", s.ID, s.Points, want.Draft.Name.Get(spec.DefaultLocale))
			if *dry {
				updated++
				continue
			}
			want.ID = have.ID
			want.Token = have.Token
			if _, err := c.UpdateAchievement(ctx, have.ID, want); err != nil {
				return fmt.Errorf("update %s: %w", s.ID, err)
			}
			lock[s.ID] = have.ID
			updated++
			if *icons && s.Icon != "" {
				if err := uploadIcon(ctx, c, have.ID, specDir, s.Icon, out); err != nil {
					return err
				}
			}
			if *icons && s.Icon != "" {
				if err := uploadIcon(ctx, c, have.ID, specDir, s.Icon, out); err != nil {
					return err
				}
			}
		default:
			unchanged++
			// An icon is not part of the text comparison, so an achievement
			// whose wording never changed can still be missing its picture.
			if *icons && s.Icon != "" && (*forceIcons || have.Draft == nil || have.Draft.IconURL == "") && !*dry {
				if err := uploadIcon(ctx, c, have.ID, specDir, s.Icon, out); err != nil {
					return err
				}
			}
		}
	}

	mine := make(map[string]bool, len(seen))
	for id := range seen {
		if gid, ok := lock[id]; ok {
			mine[gid] = true
		}
	}
	var extra []play.Achievement
	for _, a := range existing {
		if !mine[a.ID] {
			extra = append(extra, a)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].ID < extra[j].ID })
	for _, a := range extra {
		label := a.ID
		if a.Draft != nil {
			label = a.Draft.Name.Get(spec.DefaultLocale)
		}
		if !*prune {
			fmt.Fprintf(out, "extra   %-20s %s — in the console but not in the spec (use --prune to delete)\n", a.ID, label)
			continue
		}
		fmt.Fprintf(out, "delete  %-20s %s\n", a.ID, label)
		if *dry {
			continue
		}
		if err := c.DeleteAchievement(ctx, a.ID); err != nil {
			return fmt.Errorf("delete %s: %w", a.ID, err)
		}
		delete(lock, keyForID(lock, a.ID))
	}

	if !*dry {
		if err := saveLock(lockPath, lock); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "\n%d created, %d updated, %d unchanged, %d points of %d\n",
		created, updated, unchanged, total, play.PointsLimit)
	if *dry {
		fmt.Fprintln(out, "--dry-run: nothing was sent to Google")
		return nil
	}
	fmt.Fprintln(out, "Drafts saved. Players see them only after the game project is published:")
	fmt.Fprintf(out, "  droidship gplay achievements publish --app %s\n", spec.Application)
	return nil
}

func cmdAchievementsDelete(ctx context.Context, c *play.Client, args []string) error {
	fs := newFlagSet("achievements delete")
	id := fs.String("id", "", "achievement id (not the vendor token)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	if err := c.DeleteAchievement(ctx, *id); err != nil {
		return err
	}
	fmt.Println("deleted", *id)
	return nil
}

func cmdAchievementsPublish(ctx context.Context, c *play.Client, args []string) error {
	fs := newFlagSet("achievements publish")
	app := fs.String("app", "", "game project id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *app == "" {
		return fmt.Errorf("--app is required")
	}
	if err := c.PublishGameProject(ctx, *app); err != nil {
		return err
	}
	fmt.Println("game project", *app, "published — drafts are now live for players")
	return nil
}

// toAchievement turns one spec entry into the API resource.
func (s AchievementSpec) toAchievement(primary string, rank int64) (play.Achievement, error) {
	if primary == "" {
		primary = "en-US"
	}
	name, err := play.NewBundle(primary, s.Name)
	if err != nil {
		return play.Achievement{}, fmt.Errorf("name: %w", err)
	}
	desc, err := play.NewBundle(primary, s.Description)
	if err != nil {
		return play.Achievement{}, fmt.Errorf("description: %w", err)
	}
	if s.Points <= 0 || s.Points%5 != 0 {
		return play.Achievement{}, fmt.Errorf("points must be a positive multiple of five, got %d", s.Points)
	}
	a := play.Achievement{
		AchievementType: play.AchievementStandard,
		InitialState:    play.AchievementRevealed,
		Draft: &play.AchievementDetail{
			Name:        name,
			Description: desc,
			PointValue:  s.Points,
			SortRank:    rank,
		},
	}
	if s.Steps > 0 {
		a.AchievementType = play.AchievementIncremental
		a.StepsToUnlock = s.Steps
	}
	if s.Hidden {
		a.InitialState = play.AchievementHidden
	}
	return a, nil
}

// achievementChanged compares only what the spec owns, so a console-side icon
// or a published copy does not look like a difference.
func achievementChanged(have, want play.Achievement) bool {
	if have.Draft == nil {
		return true
	}
	return have.AchievementType != want.AchievementType ||
		have.InitialState != want.InitialState ||
		have.StepsToUnlock != want.StepsToUnlock ||
		have.Draft.PointValue != want.Draft.PointValue ||
		have.Draft.SortRank != want.Draft.SortRank ||
		!sameBundle(have.Draft.Name, want.Draft.Name) ||
		!sameBundle(have.Draft.Description, want.Draft.Description)
}

// checkImmutable refuses the two changes Google silently ignores.
func checkImmutable(have, want play.Achievement) error {
	if have.AchievementType != want.AchievementType {
		return fmt.Errorf("an achievement cannot change between standard and incremental; delete it and create a new one")
	}
	if have.AchievementType == play.AchievementIncremental && have.StepsToUnlock != want.StepsToUnlock {
		return fmt.Errorf("stepsToUnlock is fixed at creation (%d in the console, %d in the spec)", have.StepsToUnlock, want.StepsToUnlock)
	}
	return nil
}

func sameBundle(a, b play.LocalizedStringBundle) bool {
	if len(a.Translations) != len(b.Translations) {
		return false
	}
	seen := make(map[string]string, len(a.Translations))
	for _, t := range a.Translations {
		seen[t.Locale] = t.Value
	}
	for _, t := range b.Translations {
		if v, ok := seen[t.Locale]; !ok || v != t.Value {
			return false
		}
	}
	return true
}

func uploadIcon(ctx context.Context, c *play.Client, achievementID, dir, icon string, out io.Writer) error {
	path := icon
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, icon)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("icon %s: %w", icon, err)
	}
	ct := "image/png"
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".jpg" || ext == ".jpeg" {
		ct = "image/jpeg"
	}
	if err := c.UploadAchievementIcon(ctx, achievementID, data, ct); err != nil {
		return fmt.Errorf("icon %s: %w", icon, err)
	}
	fmt.Fprintf(out, "  icon  %s\n", icon)
	return nil
}

// The Games Configuration API assigns its own opaque id and token to every
// achievement and ignores the vendor id we send, so the mapping from our spec
// ids to Google's ids lives in a lock file next to the spec. It is meant to be
// committed: without it a second sync would create duplicates.

func lockFileFor(specPath string) string {
	ext := filepath.Ext(specPath)
	return strings.TrimSuffix(specPath, ext) + ".lock.json"
}

func loadLock(path string) (map[string]string, error) {
	lock := map[string]string{}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return lock, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return lock, nil
}

func saveLock(path string, lock map[string]string) error {
	raw, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func keyForID(lock map[string]string, gid string) string {
	for k, v := range lock {
		if v == gid {
			return k
		}
	}
	return ""
}

// indexAchievements maps our spec ids onto the console's achievements through
// the lock file, and also returns them keyed by primary-locale name so an
// achievement made by hand in the console is adopted instead of duplicated.
func indexAchievements(existing []play.Achievement, lock map[string]string, locale string) (map[string]play.Achievement, map[string]play.Achievement) {
	byID := make(map[string]play.Achievement, len(existing))
	byName := make(map[string]play.Achievement, len(existing))
	for _, a := range existing {
		byID[a.ID] = a
		if a.Draft != nil {
			byName[a.Draft.Name.Get(locale)] = a
		}
	}
	index := make(map[string]play.Achievement, len(existing))
	for specID, gid := range lock {
		if a, ok := byID[gid]; ok {
			index[specID] = a
		}
	}
	return index, byName
}

func loadAchievementsSpec(path string) (AchievementsSpec, error) {
	var spec AchievementsSpec
	raw, err := os.ReadFile(path)
	if err != nil {
		return spec, err
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return spec, fmt.Errorf("%s: %w", path, err)
	}
	if len(spec.Achievements) == 0 {
		return spec, fmt.Errorf("%s: no achievements in the spec", path)
	}
	return spec, nil
}

// newFlagSet is packageAndFlags for the commands that take no package name:
// a game project is addressed by its own numeric id, not by an applicationId.
func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}
