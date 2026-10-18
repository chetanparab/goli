package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/chetanparab/goli/internal/bookmarks"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/links"
)

// cmdImport turns a browser's bookmark export into links: every browser can
// export the Netscape HTML file, and Chrome/Edge/Brave keep a JSON file
// (e.g. ~/Library/Application Support/Google/Chrome/Default/Bookmarks).
func cmdImport(cfg *config.Config, store *links.Store, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	folder := fs.String("folder", "", "only this bookmark folder (and its subfolders), e.g. \"Bookmarks Bar/Work\"")
	tag := fs.String("t", "imported", "tag added to every imported link (\"\" for none)")
	dry := fs.Bool("dry-run", false, "show what would be created without writing anything")
	yes := fs.Bool("y", false, "don't ask for confirmation")
	// Flags may follow the file name: goli import bookmarks.html -dry-run
	var files []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(files) != 1 {
		return errors.New("usage: goli import <bookmarks.html | Chrome Bookmarks file> [-folder NAME] [-dry-run] [-y]")
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	all, err := bookmarks.Parse(data)
	if err != nil {
		return err
	}
	var picked []bookmarks.Bookmark
	for _, b := range all {
		if *folder == "" || b.Folder == *folder || strings.HasPrefix(b.Folder, *folder+"/") {
			picked = append(picked, b)
		}
	}
	if len(picked) == 0 {
		if *folder != "" {
			return fmt.Errorf("no bookmarks in folder %q (folders seen: %s)", *folder, strings.Join(folders(all), ", "))
		}
		return errors.New("no bookmarks found in that file")
	}

	// Skip URLs that already have a link; give the rest unique keys.
	have := map[string]string{}
	for _, l := range store.All() {
		have[l.URL] = l.Key
	}
	taken := map[string]bool{}
	for _, l := range store.All() {
		taken[l.Key] = true
		for _, a := range l.Aliases {
			taken[a] = true
		}
	}
	var plan []*links.Link
	skipped := 0
	for _, b := range picked {
		if _, dup := have[b.URL]; dup {
			skipped++
			continue
		}
		key := bookmarks.Key(b)
		if links.ValidateKey(key) != nil {
			skipped++
			continue
		}
		for i := 2; taken[key]; i++ {
			key = fmt.Sprintf("%s-%d", bookmarks.Key(b), i)
		}
		taken[key] = true
		l := &links.Link{Key: key, URL: b.URL, Desc: b.Title}
		if *tag != "" {
			l.Tags = []string{*tag}
		}
		plan = append(plan, l)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, l := range plan {
		fmt.Fprintf(tw, "%s/%s\t%s\t%s\n", cfg.Name(), l.Key, l.URL, l.Desc)
	}
	tw.Flush()
	fmt.Printf("\n%d links to create", len(plan))
	if skipped > 0 {
		fmt.Printf(", %d skipped (already linked, or no usable name)", skipped)
	}
	fmt.Println(".")
	if len(plan) == 0 || *dry {
		return nil
	}
	if !*yes {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("not a terminal: pass -y to import without confirmation, or -dry-run to just look")
		}
		a := ask(bufio.NewReader(os.Stdin), "Create them? You can rename any key later with `goli add` [Y/n]", "y")
		if !strings.EqualFold(a, "y") && !strings.EqualFold(a, "yes") {
			return errors.New("nothing imported")
		}
	}
	for _, l := range plan {
		if err := store.Put(l); err != nil {
			return fmt.Errorf("%s: %w", l.Key, err)
		}
	}
	fmt.Printf("imported %d links. Try %s/%s\n", len(plan), cfg.Name(), plan[0].Key)
	return nil
}

func folders(bs []bookmarks.Bookmark) []string {
	seen := map[string]bool{}
	var out []string
	for _, b := range bs {
		if b.Folder != "" && !seen[b.Folder] {
			seen[b.Folder] = true
			out = append(out, b.Folder)
		}
	}
	return out
}
