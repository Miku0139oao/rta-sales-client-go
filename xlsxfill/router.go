package xlsxfill

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

// ProviderRouter routes each workbook business store to its configured sales
// provider. It is useful when different RTA accounts cover different store
// sets. The input map is copied during construction.
type ProviderRouter struct {
	routes map[string]ProviderRoute
}

// ProviderRoute associates a store with an account-scoped provider. Profile is
// an optional display label kept only in memory. Lane identifies providers that
// must never be queried concurrently; when empty it is derived from Provider.
type ProviderRoute struct {
	Provider SalesProvider `json:"-"`
	Profile  string        `json:"-"`
	Lane     string        `json:"-"`
}

// NewProviderRouter creates an exact-match, store-scoped provider router.
func NewProviderRouter(providers map[string]SalesProvider) (*ProviderRouter, error) {
	if len(providers) == 0 {
		return nil, &rtasales.InputError{Field: "providers", Message: "at least one store provider is required"}
	}
	routes := make(map[string]ProviderRoute, len(providers))
	keys := make([]string, 0, len(providers))
	for storeID := range providers {
		keys = append(keys, storeID)
	}
	sort.Strings(keys)
	for _, rawStoreID := range keys {
		provider := providers[rawStoreID]
		storeID := strings.TrimSpace(rawStoreID)
		if storeID == "" {
			return nil, &rtasales.InputError{Field: "providers", Message: "store IDs must not be empty"}
		}
		if nilSalesProvider(provider) {
			return nil, &rtasales.InputError{Field: "providers", Message: fmt.Sprintf("provider for store %q is nil", storeID)}
		}
		if _, exists := routes[storeID]; exists {
			return nil, &rtasales.InputError{Field: "providers", Message: fmt.Sprintf("store %q is configured more than once after trimming", storeID)}
		}
		routes[storeID] = ProviderRoute{Provider: provider, Lane: providerLane(provider)}
	}
	return &ProviderRouter{routes: routes}, nil
}

// NewProfiledProviderRouter creates a router with optional display profiles and
// explicit serialization lanes. It copies every route and never serializes the
// supplied profile/lane metadata.
func NewProfiledProviderRouter(routes map[string]ProviderRoute) (*ProviderRouter, error) {
	if len(routes) == 0 {
		return nil, &rtasales.InputError{Field: "routes", Message: "at least one store provider is required"}
	}
	copyOfRoutes := make(map[string]ProviderRoute, len(routes))
	keys := make([]string, 0, len(routes))
	for storeID := range routes {
		keys = append(keys, storeID)
	}
	sort.Strings(keys)
	for _, rawStoreID := range keys {
		route := routes[rawStoreID]
		storeID := strings.TrimSpace(rawStoreID)
		if storeID == "" {
			return nil, &rtasales.InputError{Field: "routes", Message: "store IDs must not be empty"}
		}
		if nilSalesProvider(route.Provider) {
			return nil, &rtasales.InputError{Field: "routes", Message: fmt.Sprintf("provider for store %q is nil", storeID)}
		}
		if _, exists := copyOfRoutes[storeID]; exists {
			return nil, &rtasales.InputError{Field: "routes", Message: fmt.Sprintf("store %q is configured more than once after trimming", storeID)}
		}
		route.Profile = strings.TrimSpace(route.Profile)
		route.Lane = strings.TrimSpace(route.Lane)
		if route.Lane == "" {
			route.Lane = providerLane(route.Provider)
		}
		copyOfRoutes[storeID] = route
	}
	return &ProviderRouter{routes: copyOfRoutes}, nil
}

// NewProviderRouterWithProfiles is a descriptive alias for
// NewProfiledProviderRouter.
func NewProviderRouterWithProfiles(routes map[string]ProviderRoute) (*ProviderRouter, error) {
	return NewProfiledProviderRouter(routes)
}

// Sales dispatches a query only to the provider registered for its exact
// business store ID.
func (r *ProviderRouter) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	if r == nil {
		return nil, &rtasales.InputError{Field: "provider", Message: "router is nil"}
	}
	storeID := strings.TrimSpace(query.BusinessStoreID)
	if storeID == "" {
		return nil, &rtasales.InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	route, ok := r.routes[storeID]
	if !ok {
		return nil, &rtasales.StoreNotFoundError{BusinessStoreID: storeID}
	}
	query.BusinessStoreID = storeID
	return route.Provider.Sales(ctx, query)
}

// ProviderForStore returns a copy of the route for exact-match inspection by
// adapters. The route contains no credentials, but callers should still keep
// its profile and lane out of logs.
func (r *ProviderRouter) ProviderForStore(businessStoreID string) (ProviderRoute, bool) {
	if r == nil {
		return ProviderRoute{}, false
	}
	route, ok := r.routes[strings.TrimSpace(businessStoreID)]
	return route, ok
}

func providerLane(provider SalesProvider) string {
	value := reflect.ValueOf(provider)
	if !value.IsValid() {
		return "provider:nil"
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		if value.IsNil() {
			return fmt.Sprintf("provider:%T:nil", provider)
		}
		return fmt.Sprintf("provider:%T:%x", provider, value.Pointer())
	default:
		// A value provider cannot expose a stable identity without inspecting its
		// fields. Group all such providers conservatively into one serial lane.
		return "provider:value"
	}
}

func nilSalesProvider(provider SalesProvider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
