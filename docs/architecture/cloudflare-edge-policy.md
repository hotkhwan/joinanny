# ANNY Cloudflare Edge Policy

Status: policy draft for joinanny.com production rollout.

Cloudflare is the edge policy layer for `joinanny.com`. It provides DNS,
SSL/TLS termination policy, edge security controls, cache behavior, redirects,
and traffic shaping before requests reach Fly.io origins.

Cloudflare must not be treated as the trading security boundary. Trading safety
still belongs inside ANNY application and execution systems: authenticated
users, signed or secret webhook validation, internal endpoint isolation, secret
management, audit logs, confirmation gates, exchange permission limits, and the
default real-trading-off controls.

## Current Recommended Toggles

| Cloudflare setting | Recommendation | Notes |
|---|---|---|
| Client-side security | Enabled | Keep browser-side protection on for public pages. |
| Speed optimizations | Enabled for landing/static pages | Use for public static pages and hashed assets. Do not use as a substitute for origin performance work. |
| Bot Fight Mode | Disabled when app/API/webhook traffic is proxied | Bot Fight Mode can challenge or block legitimate API clients, Telegram/Fly health checks, and webhooks. |
| Leaked credentials mitigation | Enable only when username/password login exists | Do not enable as a placeholder before password login is implemented and tested. |

## SSL/TLS Policy

- Use **Full (strict)** after the Fly origin certificate is valid.
- Never use **Flexible SSL**.
- Enable **Always Use HTTPS**.
- Enable **Automatic HTTPS Rewrites**.
- Set minimum TLS version to **TLS 1.2**.
- Enable **TLS 1.3**.
- Defer HSTS until production has been stable long enough to avoid locking
  users into a broken HTTPS configuration.

## DNS Layout

| Hostname | Purpose | Policy |
|---|---|---|
| `joinanny.com` | Public landing page | Proxied through Cloudflare. |
| `www.joinanny.com` | Redirect alias | Redirect permanently to `https://joinanny.com`. |
| `app.joinanny.com` | ANNY web app | Proxied only after app routes are ready. |
| `api.joinanny.com` | Backend API | Proxied with cache bypass and API-safe security rules. |
| `docs.joinanny.com` | Reserved docs host | Reserve DNS name; do not expose private docs by accident. |
| `status.joinanny.com` | Reserved status host | Reserve for external status page or public uptime later. |

All DNS records must point only to approved public origins. Do not store
Cloudflare API tokens, zone IDs, origin cert private keys, or other credentials
in the repo.

## Cache Rules

Bypass Cloudflare cache for any route that can carry user state, auth state,
webhook data, internal control traffic, or app shell behavior:

- `/api/*`
- `/auth/*`
- `/webhook/*`
- `/internal/*`
- `/app/*`

Cache static assets only. Eligible assets should be public, immutable or
versioned, and safe to serve to every visitor:

- `/assets/*`
- `/static/*`
- hashed JavaScript, CSS, image, font, and icon files

Public landing HTML may use Cloudflare speed optimizations, but should not
depend on long edge cache until release and rollback behavior is proven.

## Security Rules

- Rate limit auth endpoints such as `/auth/*` and login/registration routes.
- Rate limit waitlist and interest endpoints such as `/api/waitlist`,
  `/api/interest`, or equivalent signup routes.
- Do not challenge API or webhook traffic with Bot Fight Mode.
- Require application-level webhook secrets or signatures for webhook routes.
- Protect `/internal/*` and admin routes separately with origin-side
  authentication, authorization, and network policy where available.
- Keep admin and internal endpoints out of public navigation and public cache.
- Use Cloudflare rules as defense-in-depth, not as proof that a trading action
  is safe to execute.

## Transparency Endpoints

`/proof/*` endpoints can be public because Mission Zero proof is designed for
public verification.

Public proof pages must expose only sanitized proof data:

- mission hash
- model manifest hash
- result hash
- recorder hash
- `txHash`

Public proof pages must never expose:

- user email
- exchange account identifier
- API key or API-key-derived material
- private strategy logic, parameters, weights, prompts, or thresholds
- raw order payload
- raw Flight Recorder snapshot with sensitive fields

If a proof page needs richer operational detail, keep it behind authenticated
admin tooling and sanitize it before display.

## Production Checklist

- [ ] Fly origin certificate is valid before switching to Full (strict).
- [ ] Flexible SSL is disabled.
- [ ] Always Use HTTPS and Automatic HTTPS Rewrites are enabled.
- [ ] Minimum TLS is TLS 1.2 and TLS 1.3 is enabled.
- [ ] HSTS remains deferred until production stability is confirmed.
- [ ] `www.joinanny.com` redirects to `joinanny.com`.
- [ ] API, auth, webhook, internal, and app routes bypass cache.
- [ ] Static assets are versioned or immutable before long edge caching.
- [ ] Bot Fight Mode is disabled for proxied app/API/webhook traffic.
- [ ] Auth and waitlist endpoints have rate limits.
- [ ] Admin/internal endpoints have origin-side protection.
- [ ] `/proof/*` responses expose hashes and `txHash` only.
- [ ] No Cloudflare credentials or origin certificate private keys are in the
  repository.
- [ ] Real trading remains disabled by default; Cloudflare changes do not alter
  trading gates.
