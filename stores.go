package rtasales

import (
	"context"
	"strings"
)

// Store identifies the one business-facing store bound to a Client. The
// binding comes from the caller's private account/store mapping; it is not
// inferred from RTA's global store catalogue.
type Store struct {
	BusinessID string `json:"business_id"`
	Label      string `json:"label"`
}

// BoundStore returns the store identity assigned to this account-scoped
// client. It performs no network request and returns a value copy.
func (c *Client) BoundStore() Store {
	return c.store
}

// Stores returns only the store bound to this client. RTA's store-tree
// endpoint is a global catalogue and must not be treated as the authenticated
// account's authorization list.
func (c *Client) Stores(ctx context.Context) ([]Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []Store{c.store}, nil
}

// RefreshStores is kept for API compatibility. Account/store bindings are
// caller-owned configuration, so refreshing returns the same single binding.
func (c *Client) RefreshStores(ctx context.Context) ([]Store, error) {
	return c.Stores(ctx)
}

func (c *Client) resolveStore(ctx context.Context, businessID string) (Store, error) {
	if err := ctx.Err(); err != nil {
		return Store{}, err
	}
	businessID = strings.TrimSpace(businessID)
	if businessID == "" {
		return Store{}, &InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	if businessID != c.store.BusinessID {
		return Store{}, &StoreNotFoundError{BusinessStoreID: businessID}
	}
	return c.store, nil
}
