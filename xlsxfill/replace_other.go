//go:build !windows

package xlsxfill

import "os"

func replaceWorkbookFile(source, destination string) error {
	return os.Rename(source, destination)
}
