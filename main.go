package main

import (
	"embed"
	"time"

	"github.com/jmaister/taronja-gateway/cli"
)

//go:embed webapp/dist
var webappEmbedFS embed.FS

// Injected by GoReleaser via -ldflags.
var (
	version   = "Dev"
	commit    = "none"
	date      = time.Now().Format(time.RFC3339)
	buildOS   = "unknown"
	buildArch = "unknown"
)

func main() {
	cli.Execute(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		OS:      buildOS,
		Arch:    buildArch,
	}, &webappEmbedFS)
}
