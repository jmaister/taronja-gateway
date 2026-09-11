package config

// defaultTLSRedirectPort is the plain-HTTP port the gateway listens on and
// redirects to HTTPS when TLS is enabled and RedirectPort is left unset —
// the conventional port a browser tries before ever knowing about HTTPS.
const defaultTLSRedirectPort = 80

// defaultACMECacheDir is where obtained ACME certificates and the ACME
// account key are persisted across restarts when ACMEConfig.CacheDir is
// left unset. Resolved against the working directory like any other
// relative path in this config (see config.go's TLS validation block).
const defaultACMECacheDir = ".autocert-cache"

// TLSConfig configures HTTPS termination on the gateway's main listener
// (server.port). Disabled by default — the gateway serves plain HTTP,
// identical to before TLS support existed.
//
// Exactly one certificate source must be configured when Enabled: either
// CertFile/KeyFile (a certificate you provide and renew yourself), or ACME
// (the gateway obtains and renews one automatically via Let's Encrypt or
// another ACME CA). They're mutually exclusive — see config.go's
// validation. See README.md's "TLS / HTTPS" section for the full
// certificate-format reference and a comparison of the two.
type TLSConfig struct {
	// Enabled turns on HTTPS termination. Requires either CertFile+KeyFile
	// or ACME. Default: false.
	Enabled bool `yaml:"enabled"`
	// CertFile is the path to the PEM-encoded certificate (or full chain:
	// leaf cert followed by any intermediates). Mutually exclusive with
	// ACME; required when Enabled and ACME is not set.
	CertFile string `yaml:"certFile,omitempty"`
	// KeyFile is the path to the PEM-encoded private key matching CertFile.
	// Mutually exclusive with ACME; required when Enabled and ACME is not
	// set.
	KeyFile string `yaml:"keyFile,omitempty"`
	// RedirectPort is the plain-HTTP port the gateway also listens on,
	// redirecting every request there to the HTTPS equivalent on
	// server.port. nil (the default, i.e. the key is simply absent) means
	// port 80; an explicit 0 disables the redirect listener entirely, e.g.
	// if something else already owns port 80 in front of the gateway. When
	// ACME is set, this listener also answers "http-01" domain-validation
	// challenges — see ACMEConfig's doc comment.
	RedirectPort *int `yaml:"redirectPort,omitempty"`
	// ACME, if set, has the gateway obtain and automatically renew its own
	// certificate via the ACME protocol (Let's Encrypt by default) instead
	// of reading one from CertFile/KeyFile. Mutually exclusive with those.
	ACME *ACMEConfig `yaml:"acme,omitempty"`
}

// ACMEConfig configures automatic certificate issuance and renewal via the
// ACME protocol (RFC 8555) — what Let's Encrypt and several other
// certificate authorities speak. Setting this (as server.tls.acme) is an
// alternative to providing your own CertFile/KeyFile: the gateway proves
// domain ownership itself and keeps the resulting certificate renewed for
// as long as it keeps running, with no external tool (certbot or similar)
// needed.
//
// Domain validation uses whichever of the two standard challenge types
// succeeds: "tls-alpn-01" (answered automatically on the main HTTPS
// listener itself — no extra port needed, but some networks/CDNs in front
// of the gateway don't pass the required ALPN protocol through) and
// "http-01" (answered on TLSConfig.RedirectPort, so that listener needs to
// stay enabled — the default — for http-01 to be available as a fallback).
// Wildcard domains (e.g. "*.example.com") are NOT supported: those require
// a "dns-01" challenge, which this integration doesn't implement — use the
// CertFile/KeyFile path with a DNS-capable ACME client (e.g. certbot's DNS
// plugins) for wildcards instead.
//
// The first certificate for a new domain is only requested lazily, on that
// domain's first real TLS handshake — not at gateway startup — so a
// misconfigured domain (DNS not yet pointed at this gateway, port 80/443
// unreachable from the internet) surfaces as a failed handshake for real
// clients, not a startup error. Using this feature means accepting the CA's
// Terms of Service on your behalf (there is no interactive prompt a
// long-running server could sensibly show).
type ACMEConfig struct {
	// Domains lists every hostname the gateway should obtain a certificate
	// for. Required: at least one. Must exactly match what clients connect
	// with (SNI) — no wildcards (see the type doc comment).
	Domains []string `yaml:"domains"`
	// Email is an optional contact address the CA (Let's Encrypt) can use
	// for expiry/problem notifications. Not validated locally — an invalid
	// address is only ever rejected by the CA itself, at registration time.
	Email string `yaml:"email,omitempty"`
	// CacheDir is where the obtained certificate(s) and the ACME account
	// key are persisted across restarts, so the gateway doesn't re-request
	// a certificate (and risk the CA's rate limits) on every restart.
	// Created automatically if it doesn't exist. Default when unset:
	// ".autocert-cache" (relative to the working directory, like any other
	// relative path in this config).
	CacheDir string `yaml:"cacheDir,omitempty"`
	// DirectoryURL overrides the ACME server to use. Empty (the default)
	// means Let's Encrypt's production directory. Set this to Let's
	// Encrypt's staging directory
	// (https://acme-staging-v02.api.letsencrypt.org/directory) while
	// testing a setup, to avoid burning the much stricter production rate
	// limits — staging certificates aren't trusted by real browsers, so
	// switch this back (or remove it) once things work.
	DirectoryURL string `yaml:"directoryURL,omitempty"`
}

// EffectiveRedirectPort returns the plain-HTTP redirect port that should
// actually be used: RedirectPort if set (including an explicit 0, meaning
// "no redirect listener"), otherwise defaultTLSRedirectPort (80).
func (t TLSConfig) EffectiveRedirectPort() int {
	if t.RedirectPort == nil {
		return defaultTLSRedirectPort
	}
	return *t.RedirectPort
}
