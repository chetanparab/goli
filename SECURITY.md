# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub's **[Report a vulnerability](https://github.com/chetanparab/goli/security/advisories/new)** form (Security tab → Advisories). Don't open a public issue.

Include what you found, how to reproduce it, and the impact you expect. You'll get an acknowledgement within a few days. Fixes are released as soon as practical, and you'll be credited unless you prefer otherwise.

## Supported versions

Only the latest release receives security fixes.

## Security model

goli is a personal tool that runs on your own machine. By design:

- The server answers only loopback clients plus networks you explicitly list in `allow` (for example your Tailscale tailnet).
- The `Host` header must match a configured hostname, which blocks DNS-rebinding attacks.
- State-changing requests are rejected when they come from another site.
- Redirect targets must be `http://` or `https://` URLs.
- AI search sends your query and link catalog to the configured provider only when you search; redirects never contact any external service.

Issues that require an attacker to already control your user account or your links file are generally out of scope.
