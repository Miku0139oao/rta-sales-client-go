//go:build !windows

package desktop

type nativeRuntimeChecker struct{}

func (nativeRuntimeChecker) Check() RuntimeStatus {
	return RuntimeStatus{
		Available: true,
		Message:   "WebKit is provided by the desktop shell / 桌面殼層已提供 WebKit",
	}
}
