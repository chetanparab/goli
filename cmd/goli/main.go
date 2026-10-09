// Command goli is a personal go-links server and CLI.
//
//	goli <key>[/args]     open a link (no server needed)
//	goli serve            run the go/ redirect server
//	goli help             list all commands
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/ctxinfo"
	"github.com/chetanparab/goli/internal/launchd"
	"github.com/chetanparab/goli/internal/links"
	"github.com/chetanparab/goli/internal/match"
	"github.com/chetanparab/goli/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `goli — short links for everything you use. Type go/<key> in your browser.

Usage:
  goli <key>[/args...]           open a link in your browser
  goli find <words...>           search links (uses Claude when configured)
  goli add <key> <url> [flags]   create or update a link (-d desc, -t tags)
  goli rm <key>                  delete a link
  goli ls [filter]               list links
  goli edit                      open links.yaml in $EDITOR
  goli check [-local]            find dead links
  goli ctx [set k=v | unset k]   show or change context (for context-aware links)
  goli serve                     run the go/ server in the foreground
  goli install | uninstall       start/stop the server at login (macOS)
  goli hosts [-apply]            print or add the /etc/hosts entry for go/ (apply needs sudo)
  goli doctor                    check that everything is wired up
  goli version

Files live in ~/.config/goli (override with $GOLI_HOME).
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "goli:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "version", "--version":
		fmt.Println("goli", version)
		return nil
	case "hosts":
		return cmdHosts(rest)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	store, err := links.Open(cfg.LinksPath())
	if err != nil {
		return err
	}

	switch cmd {
	case "serve":
		return cmdServe(cfg, store)
	case "add":
		return cmdAdd(store, rest)
	case "rm", "delete":
		if len(rest) != 1 {
			return errors.New("usage: goli rm <key>")
		}
		if err := store.Delete(rest[0]); err != nil {
			return err
		}
		fmt.Println("deleted go/" + links.NormalizeKey(rest[0]))
		return nil
	case "ls", "list":
		return cmdList(store, rest)
	case "find", "search", "?":
		return cmdFind(cfg, store, rest)
	case "edit":
		return cmdEdit(store)
	case "check":
		return cmdCheck(store, rest)
	case "ctx":
		return cmdCtx(cfg, rest)
	case "install":
		return cmdInstall()
	case "uninstall":
		if err := launchd.Uninstall(); err != nil {
			return err
		}
		fmt.Println("goli server removed from login items")
		return nil
	case "doctor":
		return cmdDoctor(cfg, store)
	case "open":
		if len(rest) == 0 {
			return errors.New("usage: goli open <key>")
		}
		return cmdOpen(cfg, store, rest)
	default:
		if strings.HasPrefix(cmd, "-") {
			return fmt.Errorf("unknown flag %s (see goli help)", cmd)
		}
		return cmdOpen(cfg, store, args)
	}
}

func cmdServe(cfg *config.Config, store *links.Store) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := server.New(cfg, store, log).ListenAndServe(ctx)
	if errors.Is(err, syscall.EACCES) {
		return fmt.Errorf("%w\nPort 80 needs listen \":80\" (all interfaces) on macOS; set listen in %s",
			err, filepath.Join(cfg.Dir(), "config.yaml"))
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func currentContext(cfg *config.Config) ctxinfo.Context {
	vars, _ := ctxinfo.LoadVars(cfg.ContextPath())
	return ctxinfo.Context{Vars: vars, Now: time.Now(), Networks: ctxinfo.NewDetector(cfg.Networks).Current()}
}

func cmdOpen(cfg *config.Config, store *links.Store, args []string) error {
	path := strings.Join(args, "/")
	res, err := store.Resolve(path, nil, currentContext(cfg))
	if errors.Is(err, links.ErrNotFound) {
		fmt.Fprintf(os.Stderr, "go/%s doesn't exist.\n", links.NormalizeKey(path))
		if sug := match.Rank(strings.ReplaceAll(path, "/", " "), store.All(), 5); len(sug) > 0 {
			fmt.Fprintln(os.Stderr, "Did you mean:")
			for _, s := range sug {
				fmt.Fprintf(os.Stderr, "  %-20s %s\n", s.Link.Key, s.Link.Desc)
			}
		}
		fmt.Fprintf(os.Stderr, "Create it: goli add %s <url>\n", links.NormalizeKey(path))
		os.Exit(1)
	}
	if err != nil {
		return err
	}
	fmt.Println(res.URL)
	return openBrowser(res.URL)
}

func openBrowser(u string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, u).Run()
}

func cmdAdd(store *links.Store, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	desc := fs.String("d", "", "description")
	tags := fs.String("t", "", "comma-separated tags")
	// Allow flags after positional args: goli add key url -d "..."
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != 2 {
		return errors.New(`usage: goli add <key> <url> [-d "description"] [-t tag1,tag2]`)
	}
	l := &links.Link{Key: pos[0], URL: pos[1], Desc: *desc}
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			l.Tags = append(l.Tags, strings.TrimSpace(strings.ToLower(t)))
		}
	}
	if old, ok := store.Get(l.Key); ok {
		l.Variants = old.Variants // keep hand-written variants on update
		if l.Desc == "" {
			l.Desc = old.Desc
		}
		if l.Tags == nil {
			l.Tags = old.Tags
		}
	}
	if err := store.Put(l); err != nil {
		return err
	}
	fmt.Printf("go/%s → %s\n", l.Key, l.URL)
	return nil
}

func cmdList(store *links.Store, args []string) error {
	filter := strings.ToLower(strings.Join(args, " "))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	n := 0
	for _, l := range store.All() {
		hay := strings.ToLower(l.Key + " " + l.URL + " " + l.Desc + " " + strings.Join(l.Tags, " "))
		if filter != "" && !strings.Contains(hay, filter) {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", l.Key, l.URL, l.Desc)
		n++
	}
	tw.Flush()
	if n == 0 {
		fmt.Fprintln(os.Stderr, "no links")
	}
	return nil
}

func cmdFind(cfg *config.Config, store *links.Store, args []string) error {
	q := strings.TrimSpace(strings.Join(args, " "))
	if q == "" {
		return errors.New("usage: goli find <words...>")
	}
	all := store.All()
	if cfg.AI.Enabled {
		picks, err := ai.New(cfg.AI.Model).Rank(context.Background(), q, all)
		if err == nil {
			for _, p := range picks {
				fmt.Printf("%3d%%  %-20s %s\n", int(p.Confidence*100), p.Key, p.Reason)
			}
			if len(picks) == 0 {
				fmt.Fprintln(os.Stderr, "no matches")
			}
			return nil
		}
		msg, _, _ := strings.Cut(err.Error(), "\n")
		fmt.Fprintf(os.Stderr, "(AI search unavailable, using fuzzy match: %s)\n", msg)
	}
	res := match.Rank(q, all, 10)
	for _, r := range res {
		fmt.Printf("%3d%%  %-20s %s\n", int(r.Score*100), r.Link.Key, r.Link.Desc)
	}
	if len(res) == 0 {
		fmt.Fprintln(os.Stderr, "no matches")
	}
	return nil
}

func cmdEdit(store *links.Store) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command("sh", "-c", editor+` "$1"`, "sh", store.Path())
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	_, err := links.Open(store.Path()) // validate
	return err
}

func cmdCheck(store *links.Store, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	local := fs.Bool("local", false, "also check localhost URLs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	results := links.Check(context.Background(), store.All(), *local)
	sort.SliceStable(results, func(i, j int) bool { return results[i].Status < results[j].Status })
	broken := 0
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, r := range results {
		mark := map[links.CheckStatus]string{links.StatusOK: "✓", links.StatusAuth: "🔒", links.StatusBroken: "✗", links.StatusSkipped: "·"}[r.Status]
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, r.Key, r.Detail, r.URL)
		if r.Status == links.StatusBroken {
			broken++
		}
	}
	tw.Flush()
	if broken > 0 {
		return fmt.Errorf("%d broken link(s)", broken)
	}
	return nil
}

func cmdCtx(cfg *config.Config, args []string) error {
	vars, err := ctxinfo.LoadVars(cfg.ContextPath())
	if err != nil {
		return err
	}
	if len(args) >= 2 && args[0] == "set" {
		for _, kv := range args[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("expected key=value, got %q", kv)
			}
			vars[k] = v
		}
		return ctxinfo.SaveVars(cfg.ContextPath(), vars)
	}
	if len(args) >= 2 && args[0] == "unset" {
		for _, k := range args[1:] {
			delete(vars, k)
		}
		return ctxinfo.SaveVars(cfg.ContextPath(), vars)
	}
	c := currentContext(cfg)
	fmt.Printf("time:     %s\n", c.Now.Format("Mon 15:04"))
	fmt.Printf("networks: %s\n", strings.Join(c.Networks, ", "))
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-9s %s\n", k+":", vars[k])
	}
	return nil
}

func cmdInstall() error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if bin, err = filepath.EvalSymlinks(bin); err != nil {
		return err
	}
	env := map[string]string{"PATH": "/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"}
	if h := os.Getenv("GOLI_HOME"); h != "" {
		env["GOLI_HOME"] = h
	}
	if err := launchd.Install(bin, env); err != nil {
		return err
	}
	fmt.Println("goli server installed; it starts at login. Logs:", launchd.LogPath())
	return nil
}

var hostsLines = []string{"127.0.0.1\tgo", "::1\tgo"}

func cmdHosts(args []string) error {
	fs := flag.NewFlagSet("hosts", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "append the entries to /etc/hosts (run with sudo)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return err
	}
	var missing []string
	for _, l := range hostsLines {
		if !hostsHas(string(data), l) {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		fmt.Println("/etc/hosts already maps go → this machine")
		return nil
	}
	if !*apply {
		fmt.Println("Add to /etc/hosts:")
		for _, l := range missing {
			fmt.Println("  " + l)
		}
		fmt.Println("or run: sudo goli hosts -apply")
		return nil
	}
	f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("%w (run with sudo)", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "\n# goli: go/ short links\n%s\n", strings.Join(missing, "\n")); err != nil {
		return err
	}
	_ = exec.Command("dscacheutil", "-flushcache").Run()
	fmt.Println("added go to /etc/hosts")
	return nil
}

func hostsHas(hosts, line string) bool {
	want := strings.Fields(line)
	for _, l := range strings.Split(hosts, "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		f := strings.Fields(l)
		if len(f) < 2 || f[0] != want[0] {
			continue
		}
		for _, name := range f[1:] {
			if name == want[1] {
				return true
			}
		}
	}
	return false
}

func cmdDoctor(cfg *config.Config, store *links.Store) error {
	ok := func(good bool, msg, fix string) {
		if good {
			fmt.Println("✓", msg)
		} else {
			fmt.Println("✗", msg)
			if fix != "" {
				fmt.Println("   →", fix)
			}
		}
	}
	fmt.Println("goli", version)
	ok(true, fmt.Sprintf("%d links in %s", len(store.All()), store.Path()), "")

	hosts, _ := os.ReadFile("/etc/hosts")
	ok(hostsHas(string(hosts), hostsLines[0]), "/etc/hosts maps go → 127.0.0.1", "sudo goli hosts -apply")

	plist, _ := launchd.PlistPath()
	_, err := os.Stat(plist)
	ok(err == nil, "server starts at login", "goli install")

	port := "80"
	if i := strings.LastIndex(cfg.Listen, ":"); i >= 0 {
		port = cfg.Listen[i+1:]
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+port+"/_/health", nil)
	req.Host = "go"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	up := err == nil && resp.StatusCode == 200
	if resp != nil {
		resp.Body.Close()
	}
	ok(up, "server answering on port "+port, "goli install   (or goli serve to run it in the foreground; logs: "+launchd.LogPath()+")")

	home, _ := os.UserHomeDir()
	_, profErr := os.Stat(filepath.Join(home, ".config", "anthropic"))
	hasCreds := os.Getenv("ANTHROPIC_API_KEY") != "" || profErr == nil
	switch {
	case !cfg.AI.Enabled:
		fmt.Println("· AI search disabled in config.yaml")
	default:
		ok(hasCreds, "Claude credentials found for AI search ("+cfg.AI.Model+")",
			"run `ant auth login` (works for the background server too), or export ANTHROPIC_API_KEY")
	}
	return nil
}
