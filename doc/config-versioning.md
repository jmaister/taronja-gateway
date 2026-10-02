# Config File Versioning

The config file declares a schema version, in `MAJOR.MINOR` format:

```yaml
version: "1.0"
```

There's no third, patch-level component — every actual change to the config
schema is either "the structure changed" (bump `MAJOR`, needs a real
migration) or "a field was added/removed with no structural change" (bump
`MINOR`, no migration needed at all — see below). A bare major number
(`version: 1`, no dot) is also accepted and treated as `1.0`, for backward
compatibility with configs migrated before `MINOR` versions existed.

On startup, the gateway logs the version it detected and what it currently
supports:

```
Config file version: 1.0 (current: 1.0)
```

**A file with no `version:` key at all is treated as pre-v1.0.0**, not as
already current — every config file written before this release has no
`version:` field (the field itself is new in v1.0.0), and some of those
need a real, one-time content change to keep working correctly under the
current schema (the `notification.email.smtp.*` block became
`notification.email.*` directly — see below). Add `version: "1.0"` once
you've migrated to declare your config current going forward.

**The gateway refuses to start unless the config file's version exactly
matches what the binary supports** — outdated, undeclared, *and* newer than
supported all fail the same way: a version mismatch means this binary can't
be sure it understands the file correctly, in either direction. `tg run`
(and `tg middleware list`) fail immediately with an error telling you what
to do.

An outdated or undeclared file:

```
FATAL: Failed to load configuration: config file 'config.yaml' has no declared version (treated as pre-v1.0.0), but this gateway requires version 1.0

Run this to upgrade it (it prints the migrated config; redirect it to a file):

    tg migrate --config config.yaml > config-v1.0.yaml

Then point --config at the new file.
```

A config newer than this binary supports (e.g. after a rollback to an older
gateway version):

```
FATAL: Failed to load configuration: config file 'config.yaml' declares version 1.2, newer than this gateway version supports (1.0)

This gateway binary predates that config schema version and can't guarantee it
honors every setting the file relies on. Upgrade the gateway binary to one that
supports config schema version 1.2 or newer, then try again.
```

There's no `tg migrate`-style fix for that second case — a migration only
ever moves a config forward, so nothing can downgrade one back down. The
only real remedy is upgrading the gateway binary itself.

`tg migrate` **prints** the migrated config to stdout — it never writes a
file itself, and never touches the original. Redirect the output to save
it, same as any other Unix command:

```bash
./tg migrate --config config.yaml > config-v1.1.yaml
./tg run --config config-v1.1.yaml
```

You choose the destination filename; `config-v<X.Y>.yaml` (as suggested in
the error above) is just a convention, not a requirement. A config several
versions behind is migrated one step at a time internally (e.g.
0.0→1.0→1.1), so a single `tg migrate` run always gets you all the way to
the version this build requires. Running it on a config that's already
current just prints the file back unchanged (with a note on stderr, so it
doesn't pollute the redirected output).

**Not every version bump needs a migration step.** A `MINOR` bump for a
purely additive change — a new optional field with a sensible zero-value
default — just stamps the new version onto the file with nothing else
touched; there's no content transformation to run. Real content migrations,
like the one below, are the exception, reserved for an actual structural
change (typically paired with a `MAJOR` bump).

**What actually changes for a pre-v1.0.0 config today:** the only real
content migration so far flattens a `notification.email.smtp:` block (the
original, v0.0.24-era shape) directly onto `notification.email:` —

```yaml
# before (pre-v1.0.0)
notification:
  email:
    enabled: true
    smtp:
      host: smtp.example.com
      port: 587

# after (version: "1.0")
notification:
  email:
    enabled: true
    host: smtp.example.com
    port: 587
```

— plus stamping `version: "1.0"` on the file. A config with no
`notification.email.smtp` block to begin with (most of them) only gets the
`version: "1.0"` line added; everything else, including comments and
`${VAR_NAME}` placeholders, passes through untouched.
