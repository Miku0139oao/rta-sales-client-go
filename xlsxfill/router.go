package xlsxfill

import (
	"context"
	"fmt"
	"strings"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

// ProviderRouter routes each workbook business store to its own account-bound
// sales provider. It is useful when a private mapping contains one RTA account
// per store. The input map is copied during construction.
type ProviderRouter struct {
	providers map[string]SalesProvider
}

// NewProviderRouter creates an exact-match, store-scoped provider router.
func NewProviderRouter(providers map[string]SalesProvider) (*ProviderRouter, error) {
	if len(providers) == 0 {
		return nil, &rtasales.InputError{Field: "providers", Message: "at least one store provider is required"}
	}
	copyOfProviders := make(map[string]SalesProvider, len(providers))
	for rawStoreID, provider := range providers {
		storeID := strings.TrimSpace(rawStoreID)
		if storeID == "" {
			return nil, &rtasales.InputError{Field: "providers", Message: "store IDs must not be empty"}
		}
		if provider == nil {
			return nil, &rtasales.InputError{Field: "providers", Message: fmt.Sprintf("provider for store %q is nil", storeID)}
		}
		if _, exists := copyOfProviders[storeID]; exists {
			return nil, &rtasales.InputError{Field: "providers", Message: fmt.Sprintf("store %q is configured more than once after trimming", storeID)}
		}
		copyOfProviders[storeID] = provider
	}
	return &ProviderRouter{providers: copyOfProviders}, nil
}

// Sales dispatches a query only to the provider registered for its exact
// business store ID.
func (r *ProviderRouter) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	storeID := strings.TrimSpace(query.BusinessStoreID)
	if storeID == "" {
		return nil, &rtasales.InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	provider, ok := r.providers[storeID]
	if !ok {
		return nil, &rtasales.StoreNotFoundError{BusinessStoreID: storeID}
	}
	query.BusinessStoreID = storeID
	return provider.Sales(ctx, query)
}
