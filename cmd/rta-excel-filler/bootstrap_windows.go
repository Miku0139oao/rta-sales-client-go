//go:build windows && !portable_update_smoke

package main

import (
	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func bootstrap() (*desktop.App, string, application.WindowsOptions, error) {
	app, err := desktop.NewNativeApp()
	return app, "miku0139oao-rta-excel-filler-2026", application.WindowsOptions{WndClass: "RTAExcelFillerWindow"}, err
}
