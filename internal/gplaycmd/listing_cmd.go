package gplaycmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/kirvigen/droidship/internal/play"
)

// withCommit opens an edit, lets fn change it, then commits — or discards the
// edit when fn fails or dry is set, so a failed run leaves the console clean.
func withCommit(ctx context.Context, c *play.Client, pkg string, dry bool, out io.Writer, fn func(editID string) error) error {
	edit, err := c.CreateEdit(ctx, pkg)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			if err := c.DeleteEdit(ctx, pkg, edit.ID); err == nil {
				fmt.Fprintln(out, "edit discarded — nothing changed in the Play Console")
			}
		}
	}()

	if err := fn(edit.ID); err != nil {
		return err
	}
	if dry {
		if err := c.ValidateEdit(ctx, pkg, edit.ID); err != nil {
			return fmt.Errorf("validate: %w", err)
		}
		fmt.Fprintln(out, "validation passed (--dry-run), nothing was published")
		return nil
	}
	if err := commitEdit(ctx, c, pkg, edit.ID, true, out); err != nil {
		return err
	}
	committed = true
	return nil
}

// cmdListing shows or replaces the store page text.
func cmdListing(ctx context.Context, c *play.Client, args []string) error {
	if len(args) > 0 && args[0] == "set" {
		return cmdListingSet(ctx, c, args[1:])
	}
	pkg, fs, err := packageAndFlags(args, "listing")
	if err != nil {
		return err
	}
	lang := fs.String("lang", "", "show only this language")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var listings []play.Listing
	if err := withEdit(ctx, c, pkg, func(editID string) error {
		listings, err = c.Listings(ctx, pkg, editID)
		return err
	}); err != nil {
		return err
	}
	if *lang != "" {
		var kept []play.Listing
		for _, l := range listings {
			if l.Language == *lang {
				kept = append(kept, l)
			}
		}
		listings = kept
	}
	if *asJSON {
		return printJSON(listings)
	}
	for i, l := range listings {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 72))
		}
		fmt.Printf("language         : %s\n", l.Language)
		fmt.Printf("title            : %s  (%d/%d)\n", l.Title, len([]rune(l.Title)), play.MaxTitleLen)
		fmt.Printf("shortDescription : %s  (%d/%d)\n", l.ShortDescription, len([]rune(l.ShortDescription)), play.MaxShortDescLen)
		if l.Video != "" {
			fmt.Printf("video            : %s\n", l.Video)
		}
		fmt.Printf("fullDescription  : (%d/%d)\n\n%s\n", len([]rune(l.FullDescription)), play.MaxFullDescLen, l.FullDescription)
	}
	return nil
}

// cmdListingSet replaces the text of one language, keeping untouched fields.
func cmdListingSet(ctx context.Context, c *play.Client, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship gplay listing set <package> --lang L [flags]")
	}
	pkg := args[0]
	fs := flag.NewFlagSet("listing set", flag.ContinueOnError)
	lang := fs.String("lang", defaultLang, "language tag of the listing to change")
	title := fs.String("title", "", "new title")
	short := fs.String("short", "", "new short description")
	full := fs.String("full", "", "new full description")
	fullFile := fs.String("full-file", "", "read the full description from a file")
	video := fs.String("video", "", "YouTube URL for the promo video")
	dry := fs.Bool("dry-run", false, "validate and discard, changing nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *full != "" && *fullFile != "" {
		return fmt.Errorf("use either --full or --full-file, not both")
	}
	body, err := resolveNotes(*full, *fullFile)
	if err != nil {
		return err
	}
	if *title == "" && *short == "" && body == "" && *video == "" {
		return fmt.Errorf("nothing to change — pass at least one of --title, --short, --full/--full-file, --video")
	}

	return withCommit(ctx, c, pkg, *dry, os.Stdout, func(editID string) error {
		cur, err := c.Listing(ctx, pkg, editID, *lang)
		if err != nil {
			return fmt.Errorf("read current listing for %s: %w", *lang, err)
		}
		next := *cur
		next.Language = *lang
		for _, f := range []struct {
			in  string
			dst *string
		}{
			{*title, &next.Title}, {*short, &next.ShortDescription},
			{body, &next.FullDescription}, {*video, &next.Video},
		} {
			if f.in != "" {
				*f.dst = f.in
			}
		}
		if err := next.Validate(); err != nil {
			return err
		}
		if _, err := c.UpdateListing(ctx, pkg, editID, next); err != nil {
			return err
		}
		printListingDiff(os.Stdout, *cur, next)
		return nil
	})
}

// printListingDiff shows only what actually changes, so a typo is visible
// before the commit rather than after.
func printListingDiff(out io.Writer, before, after play.Listing) {
	for _, f := range []struct{ name, was, now string }{
		{"title", before.Title, after.Title},
		{"shortDescription", before.ShortDescription, after.ShortDescription},
		{"fullDescription", before.FullDescription, after.FullDescription},
		{"video", before.Video, after.Video},
	} {
		if f.was == f.now {
			continue
		}
		fmt.Fprintf(out, "%s:\n  - %s\n  + %s\n", f.name, oneLine(f.was, 90), oneLine(f.now, 90))
	}
}

// cmdDetails shows or patches the app-level contact information.
func cmdDetails(ctx context.Context, c *play.Client, args []string) error {
	if len(args) > 0 && args[0] == "set" {
		return cmdDetailsSet(ctx, c, args[1:])
	}
	pkg, fs, err := packageAndFlags(args, "details")
	if err != nil {
		return err
	}
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var d *play.Details
	if err := withEdit(ctx, c, pkg, func(editID string) error {
		d, err = c.Details(ctx, pkg, editID)
		return err
	}); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(d)
	}
	fmt.Printf("defaultLanguage : %s\n", dash(d.DefaultLanguage))
	fmt.Printf("contactEmail    : %s\n", dash(d.ContactEmail))
	fmt.Printf("contactPhone    : %s\n", dash(d.ContactPhone))
	fmt.Printf("contactWebsite  : %s\n", dash(d.ContactWebsite))
	return nil
}

// cmdDetailsSet patches contact fields, leaving the others alone.
func cmdDetailsSet(ctx context.Context, c *play.Client, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship gplay details set <package> [flags]")
	}
	pkg := args[0]
	fs := flag.NewFlagSet("details set", flag.ContinueOnError)
	email := fs.String("email", "", "contact email shown on the store page")
	phone := fs.String("phone", "", "contact phone")
	website := fs.String("website", "", "contact website")
	dry := fs.Bool("dry-run", false, "validate and discard, changing nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *email == "" && *phone == "" && *website == "" {
		return fmt.Errorf("nothing to change — pass at least one of --email, --phone, --website")
	}

	return withCommit(ctx, c, pkg, *dry, os.Stdout, func(editID string) error {
		cur, err := c.Details(ctx, pkg, editID)
		if err != nil {
			return fmt.Errorf("read current details: %w", err)
		}
		patch := play.Details{ContactEmail: *email, ContactPhone: *phone, ContactWebsite: *website}
		if _, err := c.PatchDetails(ctx, pkg, editID, patch); err != nil {
			return err
		}
		for _, f := range []struct{ name, was, now string }{
			{"contactEmail", cur.ContactEmail, *email},
			{"contactPhone", cur.ContactPhone, *phone},
			{"contactWebsite", cur.ContactWebsite, *website},
		} {
			if f.now != "" && f.now != f.was {
				fmt.Printf("%s:\n  - %s\n  + %s\n", f.name, dash(f.was), f.now)
			}
		}
		return nil
	})
}

// cmdScreenshots lists, uploads or deletes store graphics.
func cmdScreenshots(ctx context.Context, c *play.Client, args []string) error {
	switch {
	case len(args) > 0 && args[0] == "upload":
		return cmdScreenshotsUpload(ctx, c, args[1:])
	case len(args) > 0 && args[0] == "delete":
		return cmdScreenshotsDelete(ctx, c, args[1:])
	}
	pkg, fs, err := packageAndFlags(args, "screenshots")
	if err != nil {
		return err
	}
	lang := fs.String("lang", defaultLang, "language tag")
	imgType := fs.String("type", "", "one image type; empty means all of them")
	asJSON := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	types := play.ImageTypes
	if *imgType != "" {
		if !play.ValidImageType(*imgType) {
			return fmt.Errorf("unknown --type %q; valid: %s", *imgType, strings.Join(play.ImageTypes, ", "))
		}
		types = []string{*imgType}
	}

	found := map[string][]play.Image{}
	if err := withEdit(ctx, c, pkg, func(editID string) error {
		for _, t := range types {
			imgs, err := c.Images(ctx, pkg, editID, *lang, t)
			if err != nil {
				return err
			}
			if len(imgs) > 0 {
				found[t] = imgs
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(found)
	}
	if len(found) == 0 {
		fmt.Printf("no images for %s\n", *lang)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\t#\tID\tURL")
	for _, t := range types {
		for i, img := range found[t] {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", t, i+1, img.ID, oneLine(img.URL, 60))
		}
	}
	return w.Flush()
}

// cmdScreenshotsUpload adds graphics in the order given on the command line —
// that order is the order the store shows them in.
func cmdScreenshotsUpload(ctx context.Context, c *play.Client, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship gplay screenshots upload <package> --type T FILE...")
	}
	pkg := args[0]
	fs := flag.NewFlagSet("screenshots upload", flag.ContinueOnError)
	lang := fs.String("lang", defaultLang, "language tag")
	imgType := fs.String("type", "phoneScreenshots", "image type")
	replace := fs.Bool("replace", false, "delete the existing images of this type first")
	dry := fs.Bool("dry-run", false, "validate and discard, changing nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	files := fs.Args()
	if len(files) == 0 {
		return fmt.Errorf("give at least one image file")
	}
	if !play.ValidImageType(*imgType) {
		return fmt.Errorf("unknown --type %q; valid: %s", *imgType, strings.Join(play.ImageTypes, ", "))
	}
	// Check every file before touching the console: a half-applied set of
	// screenshots is worse than none.
	for _, f := range files {
		if err := play.CheckImage(f, *imgType); err != nil {
			return err
		}
	}
	fmt.Printf("%d image(s) validated for %s/%s\n", len(files), *lang, *imgType)

	return withCommit(ctx, c, pkg, *dry, os.Stdout, func(editID string) error {
		if *replace {
			if err := c.DeleteAllImages(ctx, pkg, editID, *lang, *imgType); err != nil {
				return fmt.Errorf("clear existing %s: %w", *imgType, err)
			}
			fmt.Println("existing images cleared (--replace)")
		}
		for i, f := range files {
			fmt.Printf("uploading %d/%d %s…\n", i+1, len(files), f)
			img, err := c.UploadImage(ctx, pkg, editID, *lang, *imgType, f, os.Stdout)
			if err != nil {
				return fmt.Errorf("upload %s: %w", f, err)
			}
			fmt.Printf("  id %s\n", img.ID)
		}
		return nil
	})
}

// cmdScreenshotsDelete removes one image or all of a type.
func cmdScreenshotsDelete(ctx context.Context, c *play.Client, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("package name is required: droidship gplay screenshots delete <package> --type T (--id ID | --all)")
	}
	pkg := args[0]
	fs := flag.NewFlagSet("screenshots delete", flag.ContinueOnError)
	lang := fs.String("lang", defaultLang, "language tag")
	imgType := fs.String("type", "phoneScreenshots", "image type")
	id := fs.String("id", "", "id of one image to delete")
	all := fs.Bool("all", false, "delete every image of this type")
	dry := fs.Bool("dry-run", false, "validate and discard, changing nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if (*id == "") == !*all {
		return fmt.Errorf("provide exactly one of --id or --all")
	}
	if !play.ValidImageType(*imgType) {
		return fmt.Errorf("unknown --type %q", *imgType)
	}
	return withCommit(ctx, c, pkg, *dry, os.Stdout, func(editID string) error {
		if *all {
			if err := c.DeleteAllImages(ctx, pkg, editID, *lang, *imgType); err != nil {
				return err
			}
			fmt.Printf("all %s for %s deleted\n", *imgType, *lang)
			return nil
		}
		if err := c.DeleteImage(ctx, pkg, editID, *lang, *imgType, *id); err != nil {
			return err
		}
		fmt.Printf("image %s deleted\n", *id)
		return nil
	})
}
