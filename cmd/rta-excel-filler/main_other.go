//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "RTA Excel Filler is currently supported on Windows only. / RTA Excel Filler 目前僅支援 Windows。")
	os.Exit(1)
}
