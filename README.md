# goli

**Short links for everything you use.** Type `go/claude` in your browser and land on Claude. Type `go/?that doc with the q3 plans` and goli finds it for you.

goli ("go links", and yes, *goli dena* 😉) is a single, dependency-free binary that runs a tiny redirect server on your machine, plus a CLI to open, search and manage links from the terminal.

```text
go/claude                 → https://claude.ai
go/gh/goli/issues         → https://github.com/chetanparab/goli/issues
go/jira/ABC-123           → https://acme.atlassian.net/browse/ABC-123
go/roadmp                 → "did you mean go/roadmap?" + one-click create
go/?cloudflare workers    → AI search across your links
go/dashboard              → staging or prod, depending on your context
```

## Features

| | |
|---|---|
| **Links as code** | One readable `links.yaml`. Keep it in git, review changes in PRs, and run `goli check` to catch dead links. Edits made from the UI or CLI keep your comments and ordering. |
| **Smart fallback** | A typo or unknown key shows the closest matches and a form to create the link right there. |
| **Semantic search** | `go/?words` asks Claude (via your Claude Code login, or the API) which link you mean and jumps straight there when it's sure. Without AI, it falls back to fast local fuzzy search. |
| **Parameters** | `{1}`, `{name}`, `{*}` placeholders, or just add path segments: `go/gh/owner/repo`. |
| **Context-aware links** | One key, different destinations by environment (`goli ctx set env=staging`), network (home/office/VPN), time of day or weekday. |
| **Terminal & launcher** | `goli claude` opens a link without the server; `goli find …` searches; a Raycast script command is included. |
| **Private by default** | The server only answers requests from your own machine and rejects DNS-rebinding and cross-site writes. |

## Install

```bash
# from source (Go 1.24+)
git clone https://github.com/chetanparab/goli && cd goli
make install            # installs to ~/.local/bin/goli (make sure it's on PATH)
```

Prebuilt binaries for macOS and Linux are attached to each [release](https://github.com/chetanparab/goli/releases).

## Set up `go/`

```bash
sudo goli hosts -apply  # map the name "go" to 127.0.0.1 in /etc/hosts
goli install            # run the server at login (macOS LaunchAgent)
goli doctor             # verify everything
```

Then open **http://go/** once. After the first visit, browsers accept plain `go/claude` in the address bar.

> **Browser notes.** Chrome and Edge may treat `go/x` as a search until they've seen `http://go/` once. In Firefox, set `browser.fixup.domainwhitelist.go` to `true` in `about:config`. Safari works best with `http://go/x`.

**Why port 80 on all interfaces?** Browsers request `http://go/` on port 80. macOS lets a normal user bind port 80 only on all interfaces (`:80`), not on `127.0.0.1`. goli therefore listens on `:80` but refuses any request that doesn't come from loopback, so nothing is exposed to your network. To avoid this, set `listen: "127.0.0.1:8080"` and forward port 80 yourself with `pf`.

### Use go/ on your phone (Tailscale)

Phones can't edit `/etc/hosts`, but [Tailscale](https://tailscale.com) (free for personal use) gives your devices a private network with its own DNS:

1. Install Tailscale on the computer running goli and on your phone, signed in to the same account.
2. In the Tailscale admin console, rename the computer to `go` (MagicDNS must be on, which is the default).
3. In `~/.config/goli/config.yaml`:
   ```yaml
   hostnames: [go, localhost, 127.0.0.1, "*.ts.net"]
   allow: [tailscale]
   ```
4. Restart the server (`goli install` again, or `goli serve`), then open `http://go/` on the phone once.

**On iPhone**, Safari searches anything without a dot, so `go/claude` typed in the address bar becomes a Google search. Instead:

- **Home Screen app (best):** open `http://go/` in Safari → Share → **Add to Home Screen**. Tap the `go/` icon, type `claude` and press Go. Exact keys open straight away, and descriptions like "cloudflare dashboard" work too.
- **Type the full address:** `http://go/claude` always works.
- **Siri:** make a Shortcut with *Ask for Input* → *URL* `http://go/[Provided Input]` → *Open URLs*, and name it "go".

Only devices on your tailnet are let in; everything else on your Wi-Fi is still refused. The computer has to be awake for links to work: on macOS, add `keep_awake: true` to stop idle sleep while it's plugged in (closing the lid still sleeps it unless an external display is attached). Links that point to `localhost` open on the phone's own localhost, so they're only useful on the computer.

## Links file

`~/.config/goli/links.yaml` (see [`examples/links.yaml`](examples/links.yaml)):

```yaml
claude: https://claude.ai                         # short form
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
3. Query parameters are forwarded (`go/claude?q=hi`), except `ctx.*` which override context for that request.
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
goli install | uninstall       start/stop the server at login (macOS)
goli hosts [-apply]            print or add the /etc/hosts entry
goli doctor                    check that everything is wired up
```

## Configuration

`~/.config/goli/config.yaml` (created on first run; `$GOLI_HOME` overrides the directory):

```yaml
listen: ":80"
hostnames: [go, localhost, 127.0.0.1]   # accepted Host headers
# links_file: ~/code/goli-links/links.yaml
ai:
  enabled: true
  provider: claude-code                 # or: anthropic
  # model: claude-opus-5-5
networks:
  home:   {gateway: 192.168.1.1}          # matched by default gateway
  office: {resolves: intranet.example.com} # matched if this name resolves (VPN/office DNS)
```

### AI search

`go/?words` and `goli find` can ask Claude which link you mean. Choose an engine with `ai.provider`:

| Provider | Uses | Setup |
|---|---|---|
| `claude-code` (default) | The local [Claude Code](https://claude.com/claude-code) CLI and its login, i.e. your Claude subscription. No API key. | `claude auth login` once |
| `anthropic` | The Claude API, billed per request | `ant auth login` or `ANTHROPIC_API_KEY` |

What is sent, and only on a search: your query and the link catalog (keys, descriptions, tags, URLs). Redirects never call AI. The `claude-code` engine runs the CLI with no tools, no MCP servers, no project settings and no saved session, so it can only read the catalog it's given.

Set `ai.enabled: false` to keep everything offline; local fuzzy search still works, and it is also the automatic fallback whenever AI is unavailable.

## Integrations

- **Raycast**: add [`integrations/raycast`](integrations/raycast) as a Script Commands directory, then type `go/` → `claude`, or `?search words`.
- **Shell**: `alias g=goli` → `g claude`.

## Security model

- Requests are accepted only from loopback addresses, plus any networks you list in `allow` (e.g. your tailnet).
- The `Host` header must be one of `hostnames`, which blocks DNS-rebinding attacks from web pages.
- Writes require a same-origin `Origin` (form) or a JSON body (API), so other sites can't create or delete links.
- Only `http(s)://` destinations are allowed, and redirects are never cached (`302` + `no-store`), so editing a link takes effect immediately.

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

## License

[MIT](LICENSE)
