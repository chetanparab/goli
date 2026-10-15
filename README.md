# goli

**The internet, on first-name terms.** Type `go/standup` and land in your 10 am meeting. Forgot the name? Type `go/?q3 sales` and goli finds it for you.

**Website:** [goli.theanalogyarchitect.com](https://goli.theanalogyarchitect.com) · part of [The Analogy Architect](https://www.theanalogyarchitect.com)

goli ("go links", and yes, *goli dena* 😉) is a single static binary that runs a tiny redirect server on your machine, plus a CLI to open, search and manage links from the terminal.

```text
go/standup                → https://meet.google.com/abc-defg-hij
go/budget                 → https://docs.google.com/spreadsheets/d/1xQ7…/edit
go/pr/482                 → https://github.com/acme/app/pull/482
go/flight/6E-2134         → live status for any flight number
go/roadmp                 → "did you mean go/roadmap?" + one-click create
go/?q3 sales              → search by description (opt-in AI)
go/dashboard              → staging or prod, depending on your context
```

## Features

| | |
|---|---|
| **Links as code** | One readable `links.yaml`. Keep it in git, review changes in PRs, and run `goli check` to catch dead links. Edits made from the UI or CLI keep your ordering and almost all comments. |
| **Smart fallback** | A typo or unknown key shows the closest matches and a form to create the link right there. |
| **AI, your way** | `go/?words` finds a link by what it's about. Choose **Off** (built-in typo-tolerant search, nothing leaves your computer), **Claude Code** (your Claude subscription) or **your own Claude API key**, with `goli ai setup` or the settings page. |
| **Parameters** | `{1}`, `{name}`, `{*}` placeholders, or just add path segments: `go/gh/owner/repo`. |
| **Context-aware links** | One key, different destinations by environment (`goli ctx set env=staging`), network (home/office/VPN), time of day or weekday. |
| **Everywhere** | macOS, Windows and Linux; the terminal (`goli standup`, `goli find …`) and Raycast; and your iPhone or Android phone over Tailscale, where you can add and edit links too. |
| **Private by default** | The server only answers your own machine (and devices you allow), and refuses DNS rebinding, cross-site writes, framing and other sites triggering searches. |

## Install

goli runs on **macOS, Windows and Linux**.

```bash
go install github.com/chetanparab/goli/cmd/goli@latest   # needs Go 1.26+
```

Or download a prebuilt binary — [macOS (Apple silicon)](https://github.com/chetanparab/goli/releases/latest/download/goli_darwin_arm64.tar.gz) · [macOS (Intel)](https://github.com/chetanparab/goli/releases/latest/download/goli_darwin_amd64.tar.gz) · [Windows](https://github.com/chetanparab/goli/releases/latest/download/goli_windows_amd64.zip) · [Windows ARM](https://github.com/chetanparab/goli/releases/latest/download/goli_windows_arm64.zip) · [Linux](https://github.com/chetanparab/goli/releases/latest/download/goli_linux_amd64.tar.gz) · [Linux ARM](https://github.com/chetanparab/goli/releases/latest/download/goli_linux_arm64.tar.gz) — unpack it and put `goli` somewhere on your `PATH`. On macOS, run `xattr -d com.apple.quarantine goli` once first: the binaries aren't notarized yet, so Gatekeeper blocks them otherwise. From a clone, `make install` builds into `~/.local/bin`.

## Set up `go/`

The same three commands on every platform:

```bash
goli hosts      # prints the one command that maps "go" to this machine in your hosts file
goli install    # starts goli now and at every login
goli doctor     # checks everything
```

Then open **http://go/** once. After the first visit, browsers accept plain `go/standup` in the address bar.

| | macOS | Windows | Linux |
|---|---|---|---|
| Hosts file | `/etc/hosts` — `goli hosts` prints a `sudo tee` command | `C:\Windows\System32\drivers\etc\hosts` — `goli hosts` prints a PowerShell command to run **as Administrator** | `/etc/hosts` — `goli hosts` prints a `sudo tee` command |
| Starts at login via | a LaunchAgent | your user's *Run* registry key (no admin rights, no console window) | a systemd user service |
| Logs | `~/Library/Logs/goli.log` | `%LocalAppData%\goli\goli.log` | `journalctl --user -u goli` |
| Settings | `~/.config/goli` | `%AppData%\goli` | `~/.config/goli` |
| Port 80 | works as a normal user | works as a normal user | run `sudo setcap cap_net_bind_service=+ep $(which goli)` once |

goli never asks to run itself with admin rights: `goli hosts` prints a command for you to review and run instead.

> **Browser notes.** Chrome and Edge may treat `go/x` as a search until they've seen `http://go/` once. In Firefox, set `browser.fixup.domainwhitelist.go` to `true` in `about:config`. Safari works best with `http://go/x`.

**Which address does goli listen on?** On macOS and Linux, `:80` (all interfaces) — macOS only lets a normal user bind port 80 that way. The port is reachable from your network, but every request that isn't from this machine (or a network you `allow`) gets a 403. On Windows the default is `127.0.0.1:80`, so Windows Firewall never asks; set `listen: ":80"` if you want other devices to reach it.

After upgrading goli, run `goli restart`.

### Use go/ on your phone (Tailscale)

Phones can't edit `/etc/hosts`, but [Tailscale](https://tailscale.com) (free for personal use) gives your devices a private network with its own DNS:

1. Install Tailscale on the computer running goli (macOS, Windows or Linux) and on your phone (iPhone or Android), signed in to the same account.
2. In the Tailscale admin console, rename the computer to `go` (MagicDNS must be on, which is the default).
3. In `~/.config/goli/config.yaml`, using your own MagicDNS name from `tailscale status` (not a bare `*.ts.net`, which would trust every tailnet):
   ```yaml
   hostnames: [go, localhost, 127.0.0.1, "go.tailXXXX.ts.net"]
   allow: [tailscale]
   allow_write: true     # optional: add, edit and delete links from your phone too
   # listen: ":80"       # Windows only: the default listens on 127.0.0.1
   ```
4. Run `goli restart`, then open `http://go/` on the phone once.

**On iPhone**, Safari searches anything without a dot, so `go/standup` typed in the address bar becomes a Google search. Instead:

- **Home Screen app (best):** open `http://go/` in Safari → Share → **Add to Home Screen**. Tap the `go/` icon, type `standup` and press Go. Exact keys open straight away, and descriptions like "budget sheet" work too.
- **Type the full address:** `http://go/standup` always works.
- **Siri:** make a Shortcut with *Ask for Input* → *URL* `http://go/[Provided Input]` → *Open URLs*, and name it "go".

**On Android**, Chrome behaves the same way: open `http://go/` once, then ⋮ → **Add to Home screen**.

**Managing links from the phone:** with `allow_write: true`, the Home Screen app shows **+ New link**, and every link gets **edit** and **×** buttons. A link that doesn't exist yet can be created straight from its "doesn't exist" page. Leave `allow_write` off if anyone else shares your tailnet.

Only devices on your tailnet are let in, and only when the connection arrives on this machine's tailnet address; everything else on your Wi-Fi is still refused. Allowed devices can open and search links but not change them unless you set `allow_write: true`. If you put a local reverse proxy (or `tailscale serve`) in front of goli, requests it forwards (marked by `X-Forwarded-For`, `Forwarded`, `X-Real-IP` or `Tailscale-User-Login`) are treated like other devices: read-only unless `allow_write: true`, and never allowed on the settings page. Make sure your proxy sets one of those headers. The computer has to be awake for links to work: `keep_awake: true` stops idle sleep while goli runs (on macOS only while plugged in; closing a laptop lid still sleeps it unless an external display is attached). Links that point to `localhost` open on the phone's own localhost, so they're only useful on the computer.

## Links file

`~/.config/goli/links.yaml` (see [`examples/links.yaml`](examples/links.yaml)):

```yaml
standup: https://meet.google.com/abc-defg-hij    # short form
roadmap:                                          # full form
  url: https://docs.example.com/roadmap
  desc: Product roadmap and quarterly plans       # desc + tags improve search
  tags: [planning, docs]
jira: https://acme.atlassian.net/browse/{1}       # go/jira/ABC-123
pr: https://github.com/{owner}/{repo}/pull/{n}    # go/pr/acme/api/42
team/wiki: https://wiki.example.com               # nested keys
dashboard:                                        # context-aware
  url: https://grafana.example.com/d/prod
  variants:
    - when: {env: staging}
      url: https://grafana.example.com/d/staging
    - when: {hours: "22-07", days: "mon-fri"}
      url: https://oncall.example.com/now
```

Resolution rules:

1. The longest key that prefixes the path wins (`go/team/wiki/Page` → `team/wiki` + `/Page`).
2. Placeholders consume the remaining segments; with no placeholders, segments are appended to the URL path.
3. Query parameters are forwarded (`go/search?q=hi`), except `ctx.*` which override context for that request.
4. For context-aware links, the first variant whose `when` conditions all match wins.

`when` conditions: `env`/any variable (`a|b` for either), `network`, `hours` (`"09-18"`, wraps midnight), `days` (`"mon-fri"`, `"sat,sun"`).

### Links as code

Point goli at a git repo and every change becomes reviewable:

```yaml
# ~/.config/goli/config.yaml
links_file: ~/code/goli-links/links.yaml
```

The server reloads the file automatically when it changes, so `git pull` is enough to deploy.

## CLI

```text
goli <key>[/args...]           open a link in your browser
goli find <words...>           search links (uses Claude when configured)
goli add <key> <url> [-d desc] [-t tag1,tag2]
goli rm <key>
goli ls [filter]
goli edit                      open links.yaml in $EDITOR (validated on save)
goli check [-local]            find dead links
goli ctx [set k=v | unset k]   show or change context
goli serve                     run the server in the foreground
goli install | uninstall       start/stop the server at login (macOS, Windows, Linux)
goli restart                   restart the background server (after an upgrade)
goli hosts [-apply]            print (or, with admin rights, add) the hosts-file entry
goli ai [setup|use|key|test]   choose how AI search is powered
goli doctor                    check that everything is wired up
```

## Configuration

`~/.config/goli/config.yaml` (`%AppData%\goli\config.yaml` on Windows; created on first run; `$GOLI_HOME` overrides the directory):

```yaml
listen: ":80"                           # "127.0.0.1:80" by default on Windows
hostnames: [go, localhost, 127.0.0.1]   # accepted Host headers ("*.x" matches subdomains)
# links_file: ~/code/goli-links/links.yaml
# allow: [tailscale]                    # other devices (see "Use go/ on your phone")
# allow_write: false
# keep_awake: true                      # no idle sleep while goli runs
ai:                                     # easiest to change with `goli ai setup`
  enabled: false                        # opt in
  provider: claude-code                 # claude-code | anthropic (= `goli ai use api`)
  send_urls: host                       # host | full | none
  # model: claude-opus-5-5
networks:
  home:   {gateway: 192.168.1.1}          # matched by default gateway
  office: {resolves: intranet.example.com} # matched if this name resolves (VPN/office DNS)
```

### AI search

`go/?words` can find a link by what it's *about*. Choose how it's powered with **`goli ai setup`** (a guided terminal setup) or on goli's own settings page, **http://go/_/settings** (only on the computer running goli; other devices can't open it):

| Option | Uses | You need |
|---|---|---|
| **Off** (default) | goli's built-in search: forgives typos, matches descriptions | nothing; nothing leaves your computer |
| **Claude Code** | your Claude Code login, i.e. your Claude subscription | [Claude Code](https://claude.com/claude-code) installed and `claude auth login` |
| **Claude API key** | the Claude API, billed per search (a fraction of a cent; free credits count) | a key from [console.anthropic.com](https://console.anthropic.com/settings/keys) |

```bash
goli ai                 # show the options and which is in use
goli ai setup           # pick one, add a key if needed, run a test
goli ai use api haiku   # or: off · claude-code · api [opus|sonnet|haiku]
goli ai key             # save an API key (typed hidden)
goli ai test            # one search on three made-up links
```

**Every user connects their own account.** goli never ships anyone's login or key. A pasted API key is stored in your macOS Keychain, your Linux keyring (via `secret-tool`), on Windows a private file in `%LocalAppData%\goli\secrets`, or, when there's no keyring, a private `0600` file in goli's config folder; never in `config.yaml` or the logs. An `ant auth login` profile also works. An `ANTHROPIC_API_KEY` environment variable only works in the shell that has it (the background server started at login can't see it), so save the key with `goli ai key` instead.

What is sent, and only when you search: your query and the link catalog (keys, descriptions, tags, and each URL's host name only with `send_urls: host`, the default; `full` sends whole URLs, `none` sends none). Opening a link never calls AI, and searches triggered by other websites (for example a hidden `<img src="http://go/?…">`) fall back to local matching instead of spending AI calls. At most two AI searches run at once, and repeated queries are cached for 10 minutes. The Claude Code option runs the CLI with no tools, no MCP servers, no project settings and no saved session, so it can only read the catalog it's given.

## Integrations

- **Raycast**: add [`integrations/raycast`](integrations/raycast) as a Script Commands directory, then type `go/` → `standup`, or `?search words`.
- **Shell**: `alias g=goli` → `g standup`.

## Security model

- Requests are accepted only from loopback, plus networks you list in `allow` (and only when they arrive on an address in those networks). Allowed devices are read-only unless `allow_write: true`.
- The `Host` header must be one of `hostnames`, which blocks DNS-rebinding attacks from web pages.
- Cross-origin writes are refused using `Sec-Fetch-Site`/`Origin` (Go's `CrossOriginProtection`); the JSON API also requires `Content-Type: application/json`.
- Pages send `X-Frame-Options: DENY` and a strict `Content-Security-Policy`, so other sites can't frame goli (no clickjacking).
- Paths and queries are capped at 512 bytes; AI search runs only for searches you start yourself.
- Destinations must be `http(s)://`. Placeholders are escaped for where they sit (host, path or query), and the final URL is re-validated. Redirects are never cached (`302` + `no-store`).
- Logs record which key was opened, not the destination URL. Config and data files are created private to your user (0600/0700).
- `goli check` refuses private and loopback addresses (including via redirects) unless you pass `-local`, and tries `HEAD` before `GET`.

Found a problem? See [SECURITY.md](SECURITY.md).

## Roadmap

- MCP server so AI agents can resolve and create links
- Search sources beyond links.yaml (Notion, Google Drive, GitHub)
- Slack unfurls for `go/` links
- Click stats and "unused link" reports
- Browser extension for machines where you can't edit `/etc/hosts`

## Development

```bash
make test     # go test -race ./...
make lint     # go vet + golangci-lint
make run      # build and serve in the foreground
```

The project website lives in [`site/`](site) (plain HTML/CSS, no build step) and deploys to Cloudflare (Workers static assets, see `wrangler.jsonc`) from `.github/workflows/site.yml`. Check it with `node tools/check-site.mjs site`; `tools/stamp-site.sh` version-stamps the CSS/JS links before a deploy.

## License

goli is released under the [MIT License](LICENSE). Its binaries include third-party software (the Go runtime, a few MIT/BSD/Apache-2.0 libraries, and the OFL-licensed Fraunces and Hanken Grotesk fonts) whose notices are reproduced in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). After changing dependencies, run `make notices`.
