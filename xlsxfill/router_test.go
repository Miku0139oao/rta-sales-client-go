package xlsxfill

import (
	"context"
	"errors"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

func TestProviderRouterUsesExactStoreProvider(t *testing.T) {
	first := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"STORE_A": {Store: rtasales.Store{BusinessID: "STORE_A"}},
	}}
	second := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"STORE_B": {Store: rtasales.Store{BusinessID: "STORE_B"}},
	}}
	router, err := NewProviderRouter(map[string]SalesProvider{
		"STORE_A": first,
		"STORE_B": second,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := router.Sales(context.Background(), rtasales.SalesQuery{BusinessStoreID: "STORE_B"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Store.BusinessID != "STORE_B" {
		t.Fatalf("routed store=%q, want STORE_B", result.Store.BusinessID)
	}
}

func TestProviderRouterRejectsUnknownStore(t *testing.T) {
	router, err := NewProviderRouter(map[string]SalesProvider{
		"STORE_A": &fakeSalesProvider{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = router.Sales(context.Background(), rtasales.SalesQuery{BusinessStoreID: "STORE"})
	var missing *rtasales.StoreNotFoundError
	if !errors.As(err, &missing) || missing.BusinessStoreID != "STORE" {
		t.Fatalf("error=%T %v, want exact-match StoreNotFoundError", err, err)
	}
}

func TestProviderRouterCopiesConfiguration(t *testing.T) {
	providers := map[string]SalesProvider{"STORE_A": &fakeSalesProvider{}}
	router, err := NewProviderRouter(providers)
	if err != nil {
		t.Fatal(err)
	}
	delete(providers, "STORE_A")
	if _, err := router.Sales(context.Background(), rtasales.SalesQuery{BusinessStoreID: "STORE_A"}); err != nil {
		t.Fatal(err)
	}
}
