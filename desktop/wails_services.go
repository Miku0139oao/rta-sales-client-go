package desktop

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type wailsDialogService struct{}

func (wailsDialogService) OpenFile(ctx context.Context, options fileDialogOptions) (string, error) {
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
		DefaultFilename: options.DefaultFilename, Filters: wailsFilters(options.Filters),
	})
}

func (wailsDialogService) OpenDirectory(ctx context.Context, options fileDialogOptions) (string, error) {
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
	})
}

func (wailsDialogService) SaveFile(ctx context.Context, options fileDialogOptions) (string, error) {
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title: options.Title, DefaultDirectory: options.DefaultDirectory,
		DefaultFilename: options.DefaultFilename, Filters: wailsFilters(options.Filters),
		CanCreateDirectories: true,
	})
}

func wailsFilters(filters []fileDialogFilter) []runtime.FileFilter {
	result := make([]runtime.FileFilter, len(filters))
	for index, filter := range filters {
		result[index] = runtime.FileFilter{DisplayName: filter.DisplayName, Pattern: filter.Pattern}
	}
	return result
}

type wailsEventSink struct{}

func (wailsEventSink) Emit(ctx context.Context, name string, payload any) {
	runtime.EventsEmit(ctx, name, payload)
}
