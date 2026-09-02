package config

// defaultTLSRedirectPort is the plain-HTTP port the gateway listens on and
// redirects to HTTPS when TLS is enabled and RedirectPort is left unset —
// the conventional port a browser tries before ever knowing about HTTPS.
const defaultTLSRedirectPort = 80

// TLSConfig configures HTTPS termination on the gateway's main listener
// (server.port). Disabled by default — the gateway serves plain HTTP,
// identical to before TLS support existed.
type TLSConfig struct {
	// Enabled turns on HTTPS termination. Requires CertFile and KeyFile.
	// Default: false.
	Enabled bool `yaml:"enabled"`
	// CertFile is the path to the PEM-encoded certificate (or full chain:
	// leaf cert followed by any intermediates). Required when Enabled.
	CertFile string `yaml:"certFile"`
	// KeyFile is the path to the PEM-encoded private key matching CertFile.
	// Required when Enabled.
	KeyFile string `yaml:"keyFile"`
	// RedirectPort is the plain-HTTP port the gateway also listens on,
	// redirecting every request there to the HTTPS equivalent on
	// server.port. nil (the default, i.e. the key is simply absent) means
	// port 80; an explicit 0 disables the redirect listener entirely, e.g.
	// if something else already owns port 80 in front of the gateway.
	RedirectPort *int `yaml:"redirectPort,omitempty"`
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
