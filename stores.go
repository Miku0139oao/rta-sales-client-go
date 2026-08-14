package rtasales

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// Store is an RTA-authorized store expressed only in business-facing values.
// BusinessID is the exact identifier callers use in SalesQuery.
type Store struct {
	BusinessID string `json:"business_id"`
	Label      string `json:"label"`
}

// storeRecord keeps RTA's query-only values private. They must not be exposed
// through Store or persisted as an application-owned mapping.
type storeRecord struct {
	Store
	upstreamID string
	filterText string
}

type authorizedStoreOption struct {
	Key   flexibleString `json:"key"`
	Value string         `json:"value"`
}

// Stores returns the authenticated account's authorized stores. Results are
// cached for the life of the Client; call RefreshStores to reload them.
func (c *Client) Stores(ctx context.Context) ([]Store, error) {
	records, err := c.loadStores(ctx, false)
	if err != nil {
		return nil, err
	}
	return publicStores(records), nil
}

// RefreshStores reloads the authenticated account's authorized stores from
// RTA and replaces the cache only after a complete, valid response.
func (c *Client) RefreshStores(ctx context.Context) ([]Store, error) {
	records, err := c.loadStores(ctx, true)
	if err != nil {
		return nil, err
	}
	return publicStores(records), nil
}

func (c *Client) resolveStore(ctx context.Context, businessID string) (storeRecord, error) {
	businessID = strings.TrimSpace(businessID)
	if businessID == "" {
		return storeRecord{}, &InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	records, err := c.loadStores(ctx, false)
	if err != nil {
		return storeRecord{}, err
	}
	for _, record := range records {
		if record.BusinessID == businessID {
			return record, nil
		}
	}
	return storeRecord{}, &StoreNotFoundError{BusinessStoreID: businessID}
}

func (c *Client) loadStores(ctx context.Context, refresh bool) ([]storeRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !refresh {
		if records, ok := c.cachedStores(); ok {
			return records, nil
		}
	}

	c.storeLoadMu.Lock()
	defer c.storeLoadMu.Unlock()
	if !refresh {
		if records, ok := c.cachedStores(); ok {
			return records, nil
		}
	}

	records, err := c.fetchAuthorizedStores(ctx)
	if err != nil {
		return nil, err
	}
	c.storesMu.Lock()
	c.stores = append([]storeRecord(nil), records...)
	c.storesLoaded = true
	c.storesMu.Unlock()
	return append([]storeRecord(nil), records...), nil
}

func (c *Client) cachedStores() ([]storeRecord, bool) {
	c.storesMu.RLock()
	defer c.storesMu.RUnlock()
	if !c.storesLoaded {
		return nil, false
	}
	return append([]storeRecord(nil), c.stores...), true
}

func (c *Client) fetchAuthorizedStores(ctx context.Context) ([]storeRecord, error) {
	const operation = "load authorized stores"
	body, err := c.doAuthenticated(ctx, operation, func(ctx context.Context) (*http.Request, error) {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoints.authStores+"/appQuery/listStore", nil)
		if requestErr == nil {
			setCommonHeaders(request)
			request.Header.Set("Accept", "application/json")
			request.Header.Set("Origin", "https://partner.rta-os.com")
			request.Header.Set("Referer", "https://partner.rta-os.com/")
		}
		return request, requestErr
	})
	if err != nil {
		return nil, err
	}
	envelope, err := decodeSuccessfulEnvelope(body, operation)
	if err != nil {
		return nil, err
	}
	var upstream []authorizedStoreOption
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&upstream); err != nil {
		return nil, &ProtocolError{Operation: operation, Message: "invalid authorized-store data", Err: err}
	}

	records := make([]storeRecord, 0, len(upstream))
	byBusinessID := make(map[string]storeRecord, len(upstream))
	for _, option := range upstream {
		businessID, label, validValue := splitAuthorizedStoreValue(option.Value)
		record := storeRecord{
			Store: Store{
				BusinessID: businessID,
				Label:      label,
			},
			upstreamID: strings.TrimSpace(string(option.Key)),
			filterText: label,
		}
		if !validValue || record.upstreamID == "" {
			continue
		}
		if existing, exists := byBusinessID[record.BusinessID]; exists {
			if existing.upstreamID != record.upstreamID || existing.filterText != record.filterText {
				return nil, &ProtocolError{Operation: operation, Message: "authorized-store data contains an ambiguous business ID"}
			}
			continue
		}
		byBusinessID[record.BusinessID] = record
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, &ProtocolError{Operation: operation, Message: "authorized-store data contains no usable stores"}
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].BusinessID < records[right].BusinessID
	})
	return records, nil
}

func splitAuthorizedStoreValue(value string) (businessID, label string, ok bool) {
	label = strings.TrimSpace(value)
	businessID, storeName, found := strings.Cut(label, "-")
	businessID = strings.TrimSpace(businessID)
	return businessID, label, found && businessID != "" && strings.TrimSpace(storeName) != ""
}

func publicStores(records []storeRecord) []Store {
	stores := make([]Store, len(records))
	for index, record := range records {
		stores[index] = record.Store
	}
	return stores
}
