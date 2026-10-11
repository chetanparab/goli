// Command goli is a personal go-links server and CLI.
//
//	goli <key>[/args]     open a link (no server needed)
//	goli serve            run the go/ redirect server
//	goli help             list all commands
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/chetanparab/goli/internal/ai"
	"github.com/chetanparab/goli/internal/config"
	"github.com/chetanparab/goli/internal/ctxinfo"
	"github.com/chetanparab/goli/internal/links"
	"github.com/chetanparab/goli/internal/match"
	"github.com/chetanparab/goli/internal/platform"
	"github.com/chetanparab/goli/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...". Builds
// from `go install …@vX.Y.Z` don't pass ldflags, so fall back to the module
// version recorded in the binary.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
}

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
  goli install | uninstall       start/stop the server at login (macOS, Linux, Windows)
  goli restart                   restart the background server (after an upgrade)
  goli hosts [-apply]            print or add the hosts-file entry for go/ (apply needs admin rights)
  goli doctor                    check that everything is wired up
  goli version

Settings live in ~/.config/goli (%AppData%\goli on Windows; override with $GOLI_HOME).
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
		return cmdServe(cfg, store, rest)
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
		if err := platform.UninstallService(); err != nil {
			return err
		}
		fmt.Println("goli no longer starts at login")
		return nil
	case "restart":
		if err := platform.RestartService(); err != nil {
			return err
		}
		fmt.Println("goli server restarted")
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

// subcommands are names `goli <key>` can't open directly.
var subcommands = map[string]bool{
	"help": true, "version": true, "hosts": true, "serve": true, "add": true, "rm": true,
	"delete": true, "ls": true, "list": true, "find": true, "search": true, "edit": true,
	"check": true, "ctx": true, "install": true, "uninstall": true, "doctor": true, "open": true,
	"restart": true,
}

func cmdServe(cfg *config.Config, store *links.Store, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	background := fs.Bool("background", false, "detach from the console (used by `goli install` on Windows)")
	logFile := fs.String("log", "", "append logs to this file instead of stderr")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out := io.Writer(os.Stderr)
	if *logFile != "" {
		if err := os.MkdirAll(filepath.Dir(*logFile), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	if *background {
		platform.Detach()
		platform.WritePID()
	}
	log := slog.New(slog.NewTextHandler(out, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.KeepAwake {
		if err := platform.KeepAwake(os.Getpid()); err != nil {
			log.Warn("keep_awake unavailable", "err", err)
		} else {
			log.Info("keeping the computer awake while goli runs")
		}
	}
	err := server.New(cfg, store, log).ListenAndServe(ctx)
	if errors.Is(err, syscall.EACCES) {
		return fmt.Errorf("%w\nPort 80 needs listen \":80\" (all interfaces) on macOS; on Linux run "+
			"`sudo setcap cap_net_bind_service=+ep $(which goli)`. Or use a high port in %s",
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
	return platform.OpenURL(res.URL)
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
	if subcommands[l.Key] {
		fmt.Fprintf(os.Stderr, "note: %q is also a goli command; open it with `goli open %s`\n", l.Key, l.Key)
	}
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
	ranker, err := ai.New(cfg.AI)
	if err != nil {
		return err
	}
	if ranker != nil {
		picks, err := ranker.Rank(context.Background(), q, all)
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
	c := platform.EditCommand(os.Getenv("EDITOR"), store.Path())
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
	// Resolve symlinks, except into a Homebrew Cellar: that path changes on
	// every upgrade, while the bin/ symlink stays put.
	if resolved, err := filepath.EvalSymlinks(bin); err == nil && !strings.Contains(resolved, "/Cellar/") {
		bin = resolved
	}
	env := map[string]string{}
	if h := os.Getenv("GOLI_HOME"); h != "" {
		env["GOLI_HOME"] = h
	}
	if err := platform.InstallService(bin, env); err != nil {
		return err
	}
	fmt.Println("goli server installed; it starts at login. Logs:", platform.LogLocation())
	return nil
}

func cmdHosts(args []string) error {
	fs := flag.NewFlagSet("hosts", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "append the entries to the hosts file (needs admin rights)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := platform.HostsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var missing []string
	for _, l := range platform.HostsLines {
		if !platform.HostsHas(string(data), l) {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		fmt.Println("the hosts file already maps go → this machine")
		return nil
	}
	if !*apply {
		// Recommend a command that doesn't run goli itself with admin rights:
		// the goli binary is usually user-writable.
		fmt.Printf("Add go to %s (needs admin rights):\n  %s\n", path, platform.HostsInstructions(missing))
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("%w (run with admin rights)", err)
	}
	defer f.Close()
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	if _, err := fmt.Fprintf(f, "%s# goli: go/ short links%s%s%s", nl, nl, strings.Join(missing, nl), nl); err != nil {
		return err
	}
	platform.FlushDNS()
	fmt.Println("added go to", path)
	return nil
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

	hosts, _ := os.ReadFile(platform.HostsPath())
	ok(platform.HostsHas(string(hosts), platform.HostsLines[0]), "hosts file maps go → 127.0.0.1", "goli hosts   (prints the command to run)")
	ok(platform.ServiceInstalled(), "server starts at login ("+platform.Name+")", "goli install")

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
	ok(up, "server answering on port "+port, "goli install   (or goli serve to run it in the foreground; logs: "+platform.LogLocation()+")")

	if allowsTailscale(cfg.Allow) {
		name, err := tailscaleName()
		ok(err == nil && strings.HasPrefix(name, "go."), "Tailscale: this Mac is "+orNone(name, err),
			"install Tailscale (tailscale.com/download), sign in, then rename this machine to \"go\" in the admin console")
	}

	switch {
	case !cfg.AI.Enabled:
		fmt.Println("· AI search disabled in config.yaml")
	case cfg.AI.Provider == ai.ProviderAnthropic:
		home, _ := os.UserHomeDir()
		_, profErr := os.Stat(filepath.Join(home, ".config", "anthropic", "credentials"))
		ok(os.Getenv("ANTHROPIC_API_KEY") != "" || profErr == nil, "Claude API credentials for AI search",
			"run `ant auth login`, or export ANTHROPIC_API_KEY")
	default:
		cc := ai.NewClaudeCode(cfg.AI.ClaudeBin, cfg.AI.Model)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		in, err := cc.LoggedIn(ctx)
		cancel()
		msg := "claude CLI signed in for AI search"
		if err != nil {
			msg += " (" + err.Error() + ")"
		}
		ok(in, msg, "run `claude auth login` in a terminal")
	}
	return nil
}

func allowsTailscale(allow []string) bool {
	for _, a := range allow {
		if strings.EqualFold(strings.TrimSpace(a), "tailscale") {
			return true
		}
	}
	return false
}

// tailscaleName returns this machine's MagicDNS name, e.g. "go.tail1234.ts.net".
func tailscaleName() (string, error) {
	bin := platform.TailscaleBinary()
	if _, err := exec.LookPath(bin); err != nil {
		if _, statErr := os.Stat(bin); statErr != nil {
			return "", errors.New("tailscale not installed")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "status", "--json").Output()
	if err != nil {
		return "", errors.New("tailscale not running or not signed in")
	}
	var st struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return "", err
	}
	return strings.TrimSuffix(st.Self.DNSName, "."), nil
}

func orNone(name string, err error) string {
	if err != nil {
		return err.Error()
	}
	return name
}
