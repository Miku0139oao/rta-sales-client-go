//go:build windows

package desktop

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func runningWailsApp() (*application.App, error) {
	app := application.Get()
	if app == nil {
		return nil, errors.New("desktop application is not running")
	}
	return app, nil
}

type wailsDialogService struct{}

func (wailsDialogService) OpenFile(_ context.Context, options fileDialogOptions) (string, error) {
	app, err := runningWailsApp()
	if err != nil {
		return "", err
	}
	return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:          options.Title,
		Directory:      options.DefaultDirectory,
		CanChooseFiles: true,
		Filters:        wailsFilters(options.Filters),
	}).PromptForSingleSelection()
}

func (wailsDialogService) OpenDirectory(_ context.Context, options fileDialogOptions) (string, error) {
	app, err := runningWailsApp()
	if err != nil {
		return "", err
	}
	return app.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:                options.Title,
		Directory:            options.DefaultDirectory,
		CanChooseDirectories: true,
		CanChooseFiles:       false,
	}).PromptForSingleSelection()
}

func (wailsDialogService) SaveFile(_ context.Context, options fileDialogOptions) (string, error) {
	app, err := runningWailsApp()
	if err != nil {
		return "", err
	}
	return app.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title:                options.Title,
		Directory:            options.DefaultDirectory,
		Filename:             options.DefaultFilename,
		CanCreateDirectories: true,
		Filters:              wailsFilters(options.Filters),
	}).PromptForSingleSelection()
}

func wailsFilters(filters []fileDialogFilter) []application.FileFilter {
	result := make([]application.FileFilter, len(filters))
	for index, filter := range filters {
		result[index] = application.FileFilter{DisplayName: filter.DisplayName, Pattern: filter.Pattern}
	}
	return result
}

type wailsEventSink struct{}

func (wailsEventSink) Emit(_ context.Context, name string, payload any) {
	app := application.Get()
	if app == nil {
		return
	}
	app.Event.Emit(name, payload)
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	Start(a, ctx)
	return nil
}
