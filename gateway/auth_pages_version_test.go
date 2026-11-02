package gateway

import "testing"

func TestFooterVersion(t *testing.T) {
	old := Version
	defer func() { Version = old }()
	for in, want := range map[string]string{"1.2.3": "v1.2.3", "Dev": "Dev", "": ""} {
		Version = in
		if got := footerVersion(); got != want {
			t.Errorf("footerVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
