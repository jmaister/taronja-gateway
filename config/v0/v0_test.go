package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func flattenSMTP(raw []byte) []byte {
	return editYAML(raw, flattenNotificationEmailSMTP)
}

// --- flattenNotificationEmailSMTP ---

func TestFlattenNotificationEmailSMTP_MovesFieldsUpAndDropsSMTPKey(t *testing.T) {
	raw := []byte(`name: Test
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from: noreply@example.com
      fromName: Example
`)
	got := flattenSMTP(raw)

	var parsed struct {
		Notification struct {
			Email struct {
				Enabled  bool           `yaml:"enabled"`
				Host     string         `yaml:"host"`
				Port     int            `yaml:"port"`
				Username string         `yaml:"username"`
				Password string         `yaml:"password"`
				From     string         `yaml:"from"`
				FromName string         `yaml:"fromName"`
				SMTP     map[string]any `yaml:"smtp"`
			} `yaml:"email"`
		} `yaml:"notification"`
	}
	require.NoError(t, yaml.Unmarshal(got, &parsed))

	assert.True(t, parsed.Notification.Email.Enabled)
	assert.Equal(t, "smtp.example.com", parsed.Notification.Email.Host)
	assert.Equal(t, 587, parsed.Notification.Email.Port)
	assert.Equal(t, "${SMTP_USERNAME}", parsed.Notification.Email.Username, "env var placeholders must survive unresolved")
	assert.Equal(t, "${SMTP_PASSWORD}", parsed.Notification.Email.Password)
	assert.Equal(t, "noreply@example.com", parsed.Notification.Email.From)
	assert.Equal(t, "Example", parsed.Notification.Email.FromName)
	assert.Nil(t, parsed.Notification.Email.SMTP, "the smtp: key itself must be gone, not just emptied")
}

// TestFlattenNotificationEmailSMTP_Uses2SpaceIndent is the regression test
// for a real bug: yaml.Marshal's package-level function has no way to set
// the indent width and defaults to 4 spaces, so re-marshaling through it
// reformatted a migrated config's indentation from 2 spaces (what every
// config this project ships or generates uses) to 4 — every line the
// restructuring touched, not just the notification.email.smtp block
// itself. The fields round-tripping correctly (see
// TestFlattenNotificationEmailSMTP_MovesFieldsUpAndDropsSMTPKey) doesn't
// catch this, since unmarshaling the output is indent-width-agnostic —
// this checks the actual written bytes instead.
func TestFlattenNotificationEmailSMTP_Uses2SpaceIndent(t *testing.T) {
	raw := []byte(`name: Test
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587
`)
	got := flattenSMTP(raw)

	assert.Contains(t, string(got), "notification:\n  email:\n    enabled: true\n    host: smtp.example.com\n    port: 587\n",
		"nested keys must stay 2-space indented, not widen to yaml.v3's 4-space default")
}

func TestFlattenNotificationEmailSMTP_NoOpWhenNoSMTPBlock(t *testing.T) {
	raw := []byte("name: Test\nnotification:\n  email:\n    enabled: false\n")
	got := flattenSMTP(raw)
	assert.Equal(t, string(raw), string(got))
}

func TestFlattenNotificationEmailSMTP_NoOpWhenNoNotificationSection(t *testing.T) {
	raw := []byte("name: Test\nserver:\n  port: 8080\n")
	got := flattenSMTP(raw)
	assert.Equal(t, string(raw), string(got))
}

func TestFlattenNotificationEmailSMTP_NoOpOnMalformedYAML(t *testing.T) {
	raw := []byte("not: valid: yaml: at: all:\n\t- broken")
	got := flattenSMTP(raw)
	assert.Equal(t, string(raw), string(got), "malformed input must pass through untouched, not be mangled or panic")
}
