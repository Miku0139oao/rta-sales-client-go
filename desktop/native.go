package desktop

import (
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

// NewNativeApp wires Windows Credential Manager, DPAPI cookie persistence,
// native Wails dialogs/events, and the xlsxfill batch engine.
func NewNativeApp() (*App, error) {
	native, err := securestore.NewNative("")
	if err != nil {
		return nil, err
	}
	profiles, err := NewFileProfileRepository(native.Root)
	if err != nil {
		return nil, err
	}
	mancodes, err := NewFileManCodeRepository(native.Root)
	if err != nil {
		return nil, err
	}
	return newApp(appDependencies{
		profiles: profiles, mancodes: mancodes, credentials: native.Credentials,
		cookies: nativeCookieStore{native: native}, clients: rtaClientFactory{},
		engine: newXLSXEngine(), dialogs: wailsDialogService{}, events: wailsEventSink{},
		runtime: nativeRuntimeChecker{},
	})
}
