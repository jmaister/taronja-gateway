# TLS / HTTPS

The gateway can terminate HTTPS itself, on `server.port`. There are two ways
to give it a certificate — pick one (they're mutually exclusive; configuring
both is a config-load error):

| | [Option 1: your own certificate files](#option-1-your-own-certificate-files) | [Option 2: ACME / Let's Encrypt](#option-2-automatic-certificates-via-acme--lets-encrypt) |
|---|---|---|
| Config key | `certFile` / `keyFile` | `acme` |
| **Who obtains/renews the certificate** | **You** — `certbot`, a commercial CA, an internal PKI, etc., running independently of the gateway. The gateway never requests anything itself. | **The gateway itself**, automatically, for as long as it keeps running — no external tool, no cron job, nothing else to keep working. |
| **Network requirement** | None — works on a fully private/internal network, behind a firewall, with no public DNS at all. | The domain(s) must have **public DNS pointing at this gateway** and be **reachable from the internet** on port 80 and/or 443 — Let's Encrypt's own servers connect *to* the gateway to prove domain ownership. Won't work for internal-only services. |
| Wildcard domains (`*.example.com`) | Supported, if your certificate provider issues them | **Not supported** (needs a `dns-01` challenge; unimplemented here) |
| Best for | Internal/private services, an existing CDN- or org-issued certificate, wildcard certs | Public-facing services where you just want HTTPS with zero ongoing certificate management |

## Option 1: Your own certificate files

```yaml
server:
  port: 443
  tls:
    enabled: true
    certFile: /etc/letsencrypt/live/example.com/fullchain.pem
    keyFile: /etc/letsencrypt/live/example.com/privkey.pem
```

- `enabled`: Turn on HTTPS termination. Requires `certFile` and `keyFile` (or `acme` — see Option 2). Default: `false` (plain HTTP).
- `certFile`: Path to the PEM certificate (or full chain — leaf cert followed by any intermediates).
- `keyFile`: Path to the PEM private key matching `certFile`.
- `redirectPort`: Plain-HTTP port the gateway also listens on, redirecting every request there to the HTTPS equivalent on `server.port`. Omit for the default (80); set to `0` to disable the redirect listener entirely (e.g. if something else already owns port 80 in front of the gateway).

A bad or unparseable cert/key pair is rejected at config-load time (`tg validate` catches it before deploy), the same way a bad admin/CORS/route setting is.

### Certificate file format

`certFile` must be **PEM-encoded** (not DER/binary, not PKCS#12/`.pfx`) — a text file made of one or more blocks that look like this:

```
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAJC1HiIAZAiIMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
... (many more base64-encoded lines) ...
-----END CERTIFICATE-----
```

Give it the **full chain** — your certificate's own `CERTIFICATE` block followed immediately by every intermediate CA certificate's block, leaf first — not just the leaf alone. A browser that already trusts the intermediate (cached from visiting another site) will work either way, but a browser or API client seeing it for the first time won't be able to build a trust path to a root CA without it, and will reject the connection. This is exactly what a `fullchain.pem` from `certbot` already contains — use that file, not `cert.pem` (which certbot also writes, containing the leaf only).

`keyFile` must also be PEM-encoded, containing exactly one private key matching the certificate, in any of these forms (Go's standard library auto-detects which one it is):

```
-----BEGIN PRIVATE KEY-----        (PKCS#8 — RSA, ECDSA, or Ed25519)
-----BEGIN RSA PRIVATE KEY-----    (PKCS#1 — RSA only)
-----BEGIN EC PRIVATE KEY-----     (SEC1 — ECDSA only)
-----END ...-----
```

**A passphrase-encrypted private key is not supported** — the gateway has no way to prompt for a passphrase at startup, so a key file with a `Proc-Type: 4,ENCRYPTED` header (or PKCS#8's own encrypted form) fails to load with an unhelpful parse error, not a clear "this key needs a passphrase" message. Strip the passphrase before pointing `keyFile` at it:

```bash
openssl rsa -in encrypted-key.pem -out privkey.pem       # RSA key
openssl ec  -in encrypted-key.pem -out privkey.pem       # EC key
```

**Generating a self-signed certificate for local testing** (browsers will show a trust warning for it — that's expected; it's for testing the gateway's TLS support itself, not for production):

```bash
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout privkey.pem -out fullchain.pem -days 365 -nodes \
  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

**Verifying a cert/key pair actually match** (compares a hash of each side's public key — a mismatch here is exactly the kind of thing that produces `tls: private key does not match public key` at startup):

```bash
openssl x509 -noout -pubkey -in fullchain.pem | openssl md5
openssl pkey  -pubout       -in privkey.pem    | openssl md5
```

**Renewal is up to you.** The gateway never requests or renews a certificate in this mode; that's `certbot` (or whatever issued it)'s job, typically on its own cron job/systemd timer, same as it would be for e.g. nginx. After a renewed `certFile`/`keyFile` is in place, restart the gateway to load it. (Compare this to Option 2 below, where the gateway handles the entire renewal itself.)

## Option 2: Automatic certificates via ACME / Let's Encrypt

The gateway can obtain and renew its own certificate via the ACME protocol
(RFC 8555) — what Let's Encrypt and several other certificate authorities
speak — with no `certbot` or other external tool needed:

```yaml
server:
  port: 443
  tls:
    enabled: true
    acme:
      domains: ["example.com", "www.example.com"]
      email: admin@example.com
```

- `domains`: Every hostname the gateway should obtain a certificate for. Required — at least one. Must exactly match what clients connect with; **wildcard domains (`*.example.com`) are not supported** — those need a `dns-01` challenge, which this integration doesn't implement, so use Option 1 with a DNS-capable ACME client instead.
- `email`: Optional contact address the CA can use for expiry/problem notifications.
- `cacheDir`: Where the obtained certificate(s) and the ACME account key are persisted across restarts (created automatically). Default: `.autocert-cache` (relative to the working directory).
- `directoryURL`: Overrides the ACME server. Empty (the default) means Let's Encrypt's production directory. Point this at Let's Encrypt's **staging** directory while testing a setup, to avoid the much stricter production rate limits — staging certificates aren't trusted by real browsers, so switch back (or remove the override) once things work:
  ```yaml
      acme:
        domains: ["example.com"]
        directoryURL: https://acme-staging-v02.api.letsencrypt.org/directory
  ```

**Using this feature means accepting the CA's Terms of Service on your behalf** — there's no interactive prompt a long-running server could sensibly show, so the gateway accepts automatically, the same way every other ACME client integration (Caddy, Traefik, certbot's `--agree-tos`) does.

**Domain validation happens automatically**, via whichever of the two standard challenge types succeeds:
- **`tls-alpn-01`** — answered directly on the main HTTPS listener itself, no extra port needed. Some networks/CDNs in front of the gateway strip the ALPN protocol this needs, though, so it isn't always available.
- **`http-01`** — answered on the `redirectPort` listener (default 80, same one that redirects normal traffic to HTTPS), under `/.well-known/acme-challenge/`. This means `redirectPort` needs to stay enabled (the default) for `http-01` to be available as a fallback — setting it to `0` leaves `tls-alpn-01` as the only option.

**The first certificate for a new domain is requested lazily**, on that domain's first real TLS handshake — not at gateway startup. This means a misconfigured domain (DNS not yet pointed at this gateway, port 80/443 unreachable from the internet) surfaces as a failed handshake for a real client hitting it, not as a startup error — check the gateway's logs if HTTPS connections are failing right after enabling this. Renewal happens automatically in the background well before expiry, with no restart needed (there's no cert/key file for you to manage at all in this mode).

## A free bonus of terminating TLS yourself: TLS-level client fingerprinting

Whichever certificate source you use, enabling TLS also turns on **TLS-level JA4 fingerprinting** automatically — no extra config. Unlike [JA4H](middleware/ja4-fingerprint.md) (computed per HTTP request from header count/order, which varies constantly between a page load and its own subresource/API requests — see that page for why), TLS JA4 is computed once per TLS connection from the client's actual TLS stack (cipher suites, extensions, ALPN, TLS version): a property of the client's OS/browser/TLS library, not of any individual request, so it stays the same across every request on that connection. It's only possible because the gateway itself sees the raw `ClientHello` — this is the concrete meaning of "if we control the TLS certificates" in practice: TLS terminated by something else in front of this gateway (a CDN, a load balancer) means this gateway never sees a `ClientHello` at all.

It's exposed the same way as every other fingerprint signal — via the single `fingerprint`/`fingerprintType` pair on every session/traffic-metric row and in `X-User-Data` (`fingerprintType: "ja4_tls"` when TLS produced it) — see [doc/middleware/ja4-fingerprint.md](middleware/ja4-fingerprint.md#three-signals-one-header) for how the three fingerprinting signals get reduced to that single pair, and the [`X-User-Data` field reference](backend-integration.md#field-reference) for the exact JSON shape.
