package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	app, err := desktop.NewNativeApp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "RTA Excel Filler startup failed / 啟動失敗:", err)
		os.Exit(1)
	}
	if err := wails.Run(&options.App{
		Title:     "RTA Excel Filler",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: desktop.FrontendAssets,
		},
		BackgroundColour: options.NewRGB(247, 249, 252),
		OnStartup: func(ctx context.Context) {
			desktop.Start(app, ctx)
		},
		Bind: []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "miku0139oao-rta-excel-filler-2026",
		},
		Windows: &windowsoptions.Options{
			Theme:               windowsoptions.SystemDefault,
			WindowClassName:     "RTAExcelFillerWindow",
			WebviewUserDataPath: "",
			Messages:            bilingualWebViewMessages(),
		},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "RTA Excel Filler stopped / 程式已停止:", err)
		os.Exit(1)
	}
}

func bilingualWebViewMessages() *windowsoptions.Messages {
	messages := windowsoptions.DefaultMessages()
	messages.InstallationRequired = "Microsoft Edge WebView2 Runtime is required. Press OK to install it. / 需要 Microsoft Edge WebView2 Runtime，按「確定」開始安裝。"
	messages.UpdateRequired = "Microsoft Edge WebView2 Runtime must be updated. Press OK to continue. / Microsoft Edge WebView2 Runtime 需要更新，按「確定」繼續。"
	messages.MissingRequirements = "Missing requirement / 缺少必要元件"
	messages.Webview2NotInstalled = "WebView2 Runtime is not installed / 尚未安裝 WebView2 Runtime"
	messages.FailedToInstall = "WebView2 Runtime installation failed. Install it manually and try again. / WebView2 Runtime 安裝失敗，請手動安裝後重試。"
	messages.DownloadPage = "This portable app requires WebView2 Runtime. Press OK to open the download page. Minimum version: / 可攜版需要 WebView2 Runtime，按「確定」開啟下載頁。最低版本："
	messages.PressOKToInstall = "Press OK to install WebView2 Runtime. / 按「確定」安裝 WebView2 Runtime。"
	messages.ContactAdmin = "WebView2 Runtime is required. Contact your administrator. / 需要 WebView2 Runtime，請聯絡系統管理員。"
	messages.InvalidFixedWebview2 = "The configured WebView2 Runtime is invalid. / 指定的 WebView2 Runtime 無效。"
	messages.WebView2ProcessCrash = "WebView2 stopped unexpectedly; restart the app. / WebView2 意外停止，請重新啟動程式。"
	return messages
}
