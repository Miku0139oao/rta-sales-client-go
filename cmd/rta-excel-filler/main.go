package main

import (
	"fmt"
	"os"

	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const singleInstanceID = "miku0139oao-rta-excel-filler-2026"

func main() {
	desktopApp, err := desktop.NewNativeApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "RTA Excel Filler startup failed / 啟動失敗:", err)
		os.Exit(1)
	}

	var mainWindow *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "RTA 銷售分析",
		Description: "Desktop sales analyzer",
		Services: []application.Service{
			application.NewService(desktopApp),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(desktop.FrontendAssets),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if mainWindow == nil {
					return
				}
				mainWindow.Restore()
				mainWindow.Focus()
			},
		},
		Windows: application.WindowsOptions{
			WndClass: "RTAExcelFillerWindow",
		},
	})

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "RTA 銷售分析",
		Width:            1280,
		Height:           820,
		MinWidth:         960,
		MinHeight:        640,
		BackgroundColour: application.NewRGB(247, 249, 252),
		EnableFileDrop:   true,
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
	})
	mainWindow.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) == 0 {
			return
		}
		app.Event.Emit(desktop.FileDropEventName, files)
	})

	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "RTA Excel Filler stopped / 程式已停止:", err)
		os.Exit(1)
	}
}
