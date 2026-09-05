//go:build ignore

// Build explicitly with -tags production,native_smoke. Not a production entrypoint.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	root := flag.String("root", "", "fresh test directory")
	port := flag.Int("port", 9327, "loopback CDP port")
	flag.Parse()
	if *root == "" {
		panic("root is required")
	}
	data := filepath.Join(*root, "data")
	if err := os.MkdirAll(data, 0700); err != nil {
		panic(err)
	}
	backend, err := desktop.NewNativeSmokeApp(data)
	if err != nil {
		panic(err)
	}
	app := application.New(application.Options{Name: "RTA isolated native validation", Services: []application.Service{application.NewService(backend)}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(desktop.FrontendAssets)},
		Windows: application.WindowsOptions{WndClass: "RTANativeSmokeWindow", WebviewUserDataPath: filepath.Join(*root, "webview"), AdditionalBrowserArgs: []string{fmt.Sprintf("--remote-debugging-port=%d", *port), "--remote-debugging-address=127.0.0.1"}},
	})
	// No production single-instance identifier: never forwards to or closes the real app.
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "RTA isolated native validation", Width: 1280, Height: 900, MinWidth: 960, MinHeight: 640})
	if err := app.Run(); err != nil {
		panic(err)
	}
}
