//go:build !windows

package desktop

func nativeUpdateInstaller(string) (updateInstaller, error) { return nil, errUpdatesUnsupported }
