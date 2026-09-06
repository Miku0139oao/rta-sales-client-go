//go:build windows && native_smoke && portable_update_smoke

package main

import (
	"fmt"
	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/Miku0139oao/rta-sales-client-go/internal/buildinfo"
	"github.com/Miku0139oao/rta-sales-client-go/internal/smokefixture"
	"github.com/wailsapp/wails/v3/pkg/application"
	"os"
	"path/filepath"
)

func bootstrap() (*desktop.App, string, application.WindowsOptions, error) {
	m, err := smokefixture.Load()
	if err != nil {
		return nil, "", application.WindowsOptions{}, err
	}
	options := application.WindowsOptions{WndClass: "RTAUpdateSmoke" + m.Nonce, WebviewUserDataPath: filepath.Join(m.Root, "webview"), AdditionalBrowserArgs: []string{fmt.Sprintf("--remote-debugging-port=%d", m.Port), "--remote-debugging-address=127.0.0.1"}}
	app, err := desktop.NewPortableUpdateSmokeApp(m)
	if err == nil {
		f, e := os.OpenFile(filepath.Join(m.Root, "starts.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return nil, "", options, e
		}
		_, e = fmt.Fprintf(f, "%d %s %s\n", os.Getpid(), buildinfo.Version, m.Nonce)
		f.Close()
		if e != nil {
			return nil, "", options, e
		}
	}
	return app, "rta-update-smoke-" + m.Nonce, options, err
}
