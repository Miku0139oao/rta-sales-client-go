package desktop

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var embeddedFrontend embed.FS

// FrontendAssets is the production frontend consumed by Wails' asset server.
var FrontendAssets = mustSub(embeddedFrontend, "frontend/dist")

func mustSub(files fs.FS, directory string) fs.FS {
	result, err := fs.Sub(files, directory)
	if err != nil {
		panic(err)
	}
	return result
}
