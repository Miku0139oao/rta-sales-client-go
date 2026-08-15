//go:build !windows

package desktop

type nativeRuntimeChecker struct{}

func (nativeRuntimeChecker) Check() RuntimeStatus {
	return RuntimeStatus{
		Available: false,
		Message:   "RTA Excel Filler is currently supported on Windows only. / RTA Excel Filler 目前僅支援 Windows。",
	}
}
