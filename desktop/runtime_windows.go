//go:build windows

package desktop

import "github.com/wailsapp/go-webview2/webviewloader"

type nativeRuntimeChecker struct{}

func (nativeRuntimeChecker) Check() RuntimeStatus {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil || version == "" {
		return RuntimeStatus{
			Available: false,
			Message:   "Microsoft Edge WebView2 Runtime is required. Install it before using the portable app. / 可攜版需要 Microsoft Edge WebView2 Runtime，請先安裝後再啟動。",
		}
	}
	return RuntimeStatus{Available: true, Version: version, Message: "WebView2 Runtime is available / WebView2 Runtime 已就緒"}
}
