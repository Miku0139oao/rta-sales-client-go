//go:build windows && native_smoke && portable_update_smoke

package desktop

import (
	"errors"
	"github.com/Miku0139oao/rta-sales-client-go/internal/smokefixture"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"os"
	"path/filepath"
)

func NewPortableUpdateSmokeApp(m smokefixture.Manifest) (*App, error) {
	data := filepath.Join(m.Root, "data")
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	if _, err := m.File("data"); err != nil {
		return nil, err
	}
	marker := filepath.Join(data, "nonce")
	b, err := os.ReadFile(marker)
	if os.IsNotExist(err) {
		entries, e := os.ReadDir(data)
		if e != nil {
			return nil, e
		}
		if len(entries) != 0 {
			return nil, errors.New("unseeded smoke data not empty")
		}
		app, e := NewNativeSmokeApp(data)
		if e != nil {
			return nil, e
		}
		if e = os.WriteFile(marker, []byte(m.Nonce), 0600); e != nil {
			return nil, e
		}
		enableNativeUpdates(app)
		return app, nil
	}
	if err != nil || string(b) != m.Nonce {
		return nil, errors.New("smoke data nonce mismatch")
	}
	profiles, err := NewFileProfileRepository(data)
	if err != nil {
		return nil, err
	}
	groups, err := NewFileManCodeRepository(data)
	if err != nil {
		return nil, err
	}
	app, err := newApp(appDependencies{profiles: profiles, mancodes: groups, credentials: securestore.NewMemoryCredentialStore(), cookies: smokeCookies{}, clients: smokeClients{}, engine: newXLSXEngine(), dialogs: smokeDialogs{filepath.Join(data, "exports")}, events: wailsEventSink{}, runtime: nativeRuntimeChecker{}})
	if err == nil {
		enableNativeUpdates(app)
	}
	return app, err
}
