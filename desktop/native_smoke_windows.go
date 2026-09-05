//go:build windows && native_smoke

package desktop

// This adapter exists only in explicitly tagged validation binaries. It never
// creates an RTA client or opens the OS credential vault.
import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

type smokeClients struct{}

func (smokeClients) New(securestore.Credential, rtasales.CookieStore) (accountClient, error) {
	return smokeClient{}, nil
}

type smokeCookies struct{}

func (smokeCookies) CookieStore(string) (rtasales.CookieStore, error) {
	return &securestore.MemoryCookieStore{}, nil
}
func (smokeCookies) DeleteCookie(string) error { return nil }

type smokeDialogs struct{ directory string }

func (smokeDialogs) OpenFile(context.Context, fileDialogOptions) (string, error) { return "", nil }
func (smokeDialogs) SaveFile(context.Context, fileDialogOptions) (string, error) { return "", nil }
func (d smokeDialogs) OpenDirectory(context.Context, fileDialogOptions) (string, error) {
	return d.directory, nil
}

type smokeClient struct{}

func (smokeClient) Stores(context.Context) ([]rtasales.Store, error) {
	return []rtasales.Store{{BusinessID: "107", Label: "107 - Native synthetic"}, {BusinessID: "108", Label: "108 - Native synthetic"}}, nil
}
func (smokeClient) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Match the synthetic article totals, including the all-authorized-store lane.
	total, transactions := 21440.0, 15.0
	if query.StartDate.Month() != 8 {
		total /= 2
	}
	if query.AllStores {
		total *= 2
		transactions *= 2
	}
	result := &rtasales.SalesResult{TrendGrossSaleAmount: &total, TotalTransactionCount: &transactions}
	if query.SkipArticle {
		return result, nil
	}
	for index := 0; index < 64; index++ {
		amount := float64(65-index) * 10
		if query.StartDate.Month() != 8 {
			amount /= 2
		}
		result.Items = append(result.Items, rtasales.SaleItem{Matnr: fmt.Sprintf("00%03d", index+100), ArticleName: fmt.Sprintf("原生合成商品 %02d", index+1), BrandName: "Synthetic", PurchaseCategory1Name: "健康與美容", PurchaseCategory2Name: "肌膚護理", PurchaseCategory3Name: "日常護理", PurchaseCategory4Name: "人氣商品", PurchaseCategory5Name: "標準裝", TPSaleAmount: amount, TPGrossSaleAmount: amount, TPSaleQuantity: float64(index + 1), TPGrossSaleQuantity: float64(index + 1), TPTransactionCount: 1})
	}
	return result, nil
}

func NewNativeSmokeApp(root string) (*App, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(entries) != 0 {
		return nil, errors.New("smoke data directory must be empty")
	}
	profiles, err := NewFileProfileRepository(root)
	if err != nil {
		return nil, err
	}
	groups, err := NewFileManCodeRepository(root)
	if err != nil {
		return nil, err
	}
	exports := filepath.Join(root, "exports")
	if err := os.MkdirAll(exports, 0700); err != nil {
		return nil, err
	}
	app, err := newApp(appDependencies{profiles: profiles, mancodes: groups, credentials: securestore.NewMemoryCredentialStore(), cookies: smokeCookies{}, clients: smokeClients{}, engine: newXLSXEngine(), dialogs: smokeDialogs{exports}, events: wailsEventSink{}, runtime: nativeRuntimeChecker{}})
	if err != nil {
		return nil, err
	}
	_, err = app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "原生隔離驗證", Account: "synthetic-only", Password: "synthetic-only", Enabled: true})
	return app, err
}
