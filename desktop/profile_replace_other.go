//go:build !windows

package desktop

import "os"

func replaceProfileFile(source, destination string) error {
	return os.Rename(source, destination)
}
