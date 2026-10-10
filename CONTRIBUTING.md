# Contributing to goli

Thanks for helping! goli aims to stay small, fast and private, so please open an issue to discuss larger changes before writing code.

## Development

Requirements: Go (see `go.mod`), and optionally [golangci-lint](https://golangci-lint.run).

```bash
make test     # go test -race ./...
make lint     # go vet + golangci-lint
make run      # build and run the server in the foreground
```

To try changes without touching your real setup, point goli at a scratch directory and a high port:

```bash
export GOLI_HOME=$(mktemp -d)
printf 'listen: "127.0.0.1:8080"\nai: {enabled: false}\n' > $GOLI_HOME/config.yaml
go run ./cmd/goli serve
```

## Pull requests

- Keep PRs focused; one change per PR.
- Add or update tests for behaviour changes. `make test` and `make lint` must pass.
- Update `README.md` when you change user-facing behaviour or config.
- Never add real personal links, tokens or hostnames to examples or tests; use `example.com`.

## Reporting security issues

Please don't open public issues for vulnerabilities. See [SECURITY.md](SECURITY.md).
