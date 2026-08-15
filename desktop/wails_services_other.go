//go:build !windows

package desktop

import (
	"context"
	"errors"
)

var errWindowsDesktopOnly = errors.New("native Wails dialogs are only available on Windows")

type wailsDialogService struct{}

func (wailsDialogService) OpenFile(context.Context, fileDialogOptions) (string, error) {
	return "", errWindowsDesktopOnly
}

func (wailsDialogService) OpenDirectory(context.Context, fileDialogOptions) (string, error) {
	return "", errWindowsDesktopOnly
}

func (wailsDialogService) SaveFile(context.Context, fileDialogOptions) (string, error) {
	return "", errWindowsDesktopOnly
}

type wailsEventSink struct{}

func (wailsEventSink) Emit(context.Context, string, any) {}
