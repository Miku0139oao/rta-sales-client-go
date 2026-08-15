package desktop

import (
	"context"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

// FileDropEventName is emitted when the user drops files onto a window
// drop target. The payload is []string of absolute paths.
const FileDropEventName = "rta:file-drop"

type fileDialogOptions struct {
	Title            string
	DefaultDirectory string
	DefaultFilename  string
	Filters          []fileDialogFilter
}

type fileDialogFilter struct {
	DisplayName string
	Pattern     string
}

type dialogService interface {
	OpenFile(context.Context, fileDialogOptions) (string, error)
	OpenDirectory(context.Context, fileDialogOptions) (string, error)
	SaveFile(context.Context, fileDialogOptions) (string, error)
}

type eventSink interface {
	Emit(context.Context, string, any)
}

type runtimeChecker interface {
	Check() RuntimeStatus
}

type profileCookieStore interface {
	CookieStore(profileID string) (rtasales.CookieStore, error)
	DeleteCookie(profileID string) error
}

type nativeCookieStore struct {
	native *securestore.Native
}

func (s nativeCookieStore) CookieStore(profileID string) (rtasales.CookieStore, error) {
	return s.native.CookieStore(profileID)
}

func (s nativeCookieStore) DeleteCookie(profileID string) error {
	return s.native.DeleteCookie(profileID)
}

type accountClient interface {
	Stores(context.Context) ([]rtasales.Store, error)
	Sales(context.Context, rtasales.SalesQuery) (*rtasales.SalesResult, error)
}

type clientFactory interface {
	New(securestore.Credential, rtasales.CookieStore) (accountClient, error)
}

type rtaClientFactory struct{}

func (rtaClientFactory) New(credential securestore.Credential, cookieStore rtasales.CookieStore) (accountClient, error) {
	return rtasales.NewClient(rtasales.Config{
		Account:        credential.Account,
		Password:       credential.Password,
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		CookieStore:    cookieStore,
		LoginAttempts:  4,
		// Store jobs already run up to 32-wide. Keep page fan-out off so one
		// multi-page store does not multiply that load against RTA.
		PageConcurrency: 1,
	})
}
