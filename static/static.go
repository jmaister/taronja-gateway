package static

import "embed"

//go:embed *
var StaticAssetsFS embed.FS
