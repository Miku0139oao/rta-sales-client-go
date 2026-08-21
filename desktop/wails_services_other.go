//go:build !windows

package desktop

import (
	"context"
	"errors"
)

var errDesktopDialogsUnavailable = errors.New("native desktop dialogs are not available in the web server")

type wailsDialogService struct{}

func (wailsDialogService) OpenFile(context.Context, fileDialogOptions) (string, error) {
	return "", errDesktopDialogsUnavailable
}

func (wailsDialogService) OpenDirectory(context.Context, fileDialogOptions) (string, error) {
	return "", errDesktopDialogsUnavailable
}

func (wailsDialogService) SaveFile(context.Context, fileDialogOptions) (string, error) {
	return "", errDesktopDialogsUnavailable
}

type wailsEventSink struct{}

func (wailsEventSink) Emit(context.Context, string, any) {}
