package rtasales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Store identifies one business-facing store visible to the authenticated
// account. Upstream identifiers remain private to Client.
type Store struct {
	BusinessID string `json:"business_id"`
	Label      string `json:"label"`
}

type storeRecord struct {
	Store
	upstreamID string
}

type storeNode struct {
	Label    string         `json:"label"`
	StoreID  flexibleString `json:"storeId"`
	Value    flexibleString `json:"value"`
	Children []storeNode    `json:"children"`
	Stores   []storeNode    `json:"stores"`
}

type storeTreeData struct {
	StoreTree []storeNode `json:"storeTree"`
	Areas     []storeNode `json:"areas"`
}

// Stores returns the business-facing stores available to the authenticated
// account. Results are cached and returned as a defensive copy.
func (c *Client) Stores(ctx context.Context) ([]Store, error) {
	c.storesMu.RLock()
	if len(c.stores) > 0 {
		stores := publicStores(c.stores)
		c.storesMu.RUnlock()
		return stores, nil
	}
	c.storesMu.RUnlock()

	c.storeLoadMu.Lock()
	defer c.storeLoadMu.Unlock()
	// Another caller may have populated the cache while this caller waited.
	c.storesMu.RLock()
	if len(c.stores) > 0 {
		stores := publicStores(c.stores)
		c.storesMu.RUnlock()
		return stores, nil
	}
	c.storesMu.RUnlock()
	return c.refreshStoresLocked(ctx)
}

// RefreshStores reloads the stores available to the account. Concurrent
// callers share the refresh lock so the cache is replaced atomically.
func (c *Client) RefreshStores(ctx context.Context) ([]Store, error) {
	c.storeLoadMu.Lock()
	defer c.storeLoadMu.Unlock()
	return c.refreshStoresLocked(ctx)
}

func (c *Client) refreshStoresLocked(ctx context.Context) ([]Store, error) {
	stores, treeError := c.loadStoreTree(ctx)
	if treeError != nil || len(stores) == 0 {
		flatStores, flatError := c.loadFlatStores(ctx)
		if flatError != nil {
			if treeError != nil {
				return nil, errors.Join(treeError, flatError)
			}
			return nil, flatError
		}
		stores = flatStores
	}
	if len(stores) == 0 {
		return nil, &ProtocolError{Operation: "load stores", Message: "RTA returned no stores"}
	}
	stores, err := validateStoreRecords(stores)
	if err != nil {
		return nil, err
	}
	c.storesMu.Lock()
	c.stores = cloneStoreRecords(stores)
	c.storesMu.Unlock()
	return publicStores(stores), nil
}

func (c *Client) loadStoreTree(ctx context.Context) ([]storeRecord, error) {
	body, err := c.doAuthenticated(ctx, "load store tree", func(ctx context.Context) (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoints.stock+"/storeStock/areasAndStores", nil)
		if err == nil {
			setStoreHeaders(request)
		}
		return request, err
	})
	if err != nil {
		return nil, err
	}
	envelope, err := decodeSuccessfulEnvelope(body, "load store tree")
	if err != nil {
		return nil, err
	}
	var data storeTreeData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, &ProtocolError{Operation: "load store tree", Message: "invalid store tree data", Err: err}
	}
	stores := make([]storeRecord, 0)
	appendNodes(&stores, data.StoreTree)
	appendNodes(&stores, data.Areas)
	return stores, nil
}

func (c *Client) loadFlatStores(ctx context.Context) ([]storeRecord, error) {
	body, err := c.doAuthenticated(ctx, "load flat stores", func(ctx context.Context) (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoints.stock+"/storeStock/allStore", nil)
		if err == nil {
			setStoreHeaders(request)
		}
		return request, err
	})
	if err != nil {
		return nil, err
	}
	envelope, err := decodeSuccessfulEnvelope(body, "load flat stores")
	if err != nil {
		return nil, err
	}
	var nodes []storeNode
	if err := json.Unmarshal(envelope.Data, &nodes); err != nil {
		return nil, &ProtocolError{Operation: "load flat stores", Message: "invalid flat store data", Err: err}
	}
	stores := make([]storeRecord, 0, len(nodes))
	appendNodes(&stores, nodes)
	return stores, nil
}

func decodeSuccessfulEnvelope(body []byte, operation string) (rtaEnvelope, error) {
	envelope, err := decodeEnvelope(body, operation)
	if err != nil {
		return envelope, err
	}
	code := string(envelope.Code)
	if !successfulCode(code) {
		return envelope, &ProtocolError{Operation: operation, Message: fmt.Sprintf("RTA code %s: %s", code, envelopeMessage(envelope))}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return envelope, &ProtocolError{Operation: operation, Message: "response data is empty"}
	}
	return envelope, nil
}

func appendNodes(stores *[]storeRecord, nodes []storeNode) {
	for _, node := range nodes {
		upstreamID := strings.TrimSpace(string(node.Value))
		if upstreamID == "" {
			upstreamID = strings.TrimSpace(string(node.StoreID))
		}
		label := strings.TrimSpace(node.Label)
		businessID := businessIDFromLabel(label)
		// Area and distribution-center containers can also carry identifiers.
		// Only leaf nodes are selectable business stores.
		if len(node.Stores) == 0 && len(node.Children) == 0 && upstreamID != "" && businessID != "" {
			*stores = append(*stores, storeRecord{
				Store: Store{
					BusinessID: businessID,
					Label:      label,
				},
				upstreamID: upstreamID,
			})
		}
		appendNodes(stores, node.Stores)
		appendNodes(stores, node.Children)
	}
}

func businessIDFromLabel(label string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(label), func(character rune) bool {
		switch character {
		case '-', '－', '_', ' ', '\t', '(', '（':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func validateStoreRecords(input []storeRecord) ([]storeRecord, error) {
	seen := make(map[string]storeRecord, len(input))
	output := make([]storeRecord, 0, len(input))
	for _, record := range input {
		record.BusinessID = strings.TrimSpace(record.BusinessID)
		record.Label = strings.TrimSpace(record.Label)
		record.upstreamID = strings.TrimSpace(record.upstreamID)
		if record.BusinessID == "" || record.upstreamID == "" {
			continue
		}
		if existing, ok := seen[record.BusinessID]; ok {
			if existing.upstreamID != record.upstreamID {
				return nil, &ProtocolError{
					Operation: "load stores",
					Message:   fmt.Sprintf("business store %q has conflicting upstream records", record.BusinessID),
				}
			}
			continue
		}
		seen[record.BusinessID] = record
		output = append(output, record)
	}
	return output, nil
}

func (c *Client) resolveStore(ctx context.Context, businessID string) (storeRecord, error) {
	businessID = strings.TrimSpace(businessID)
	if businessID == "" {
		return storeRecord{}, &InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	c.storesMu.RLock()
	hadCache := len(c.stores) > 0
	cached := cloneStoreRecords(c.stores)
	c.storesMu.RUnlock()
	if !hadCache {
		if _, err := c.RefreshStores(ctx); err != nil {
			return storeRecord{}, err
		}
		c.storesMu.RLock()
		cached = cloneStoreRecords(c.stores)
		c.storesMu.RUnlock()
	}
	if store, ok := findBusinessStore(cached, businessID); ok {
		return store, nil
	}
	if hadCache {
		if _, err := c.RefreshStores(ctx); err != nil {
			return storeRecord{}, err
		}
		c.storesMu.RLock()
		refreshed := cloneStoreRecords(c.stores)
		c.storesMu.RUnlock()
		if store, ok := findBusinessStore(refreshed, businessID); ok {
			return store, nil
		}
	}
	return storeRecord{}, &StoreNotFoundError{BusinessStoreID: businessID}
}

func findBusinessStore(stores []storeRecord, wanted string) (storeRecord, bool) {
	for _, store := range stores {
		if store.BusinessID == wanted {
			return store, true
		}
	}
	return storeRecord{}, false
}

func cloneStoreRecords(stores []storeRecord) []storeRecord {
	return append([]storeRecord(nil), stores...)
}

func publicStores(records []storeRecord) []Store {
	stores := make([]Store, len(records))
	for index, record := range records {
		stores[index] = record.Store
	}
	return stores
}

func setStoreHeaders(request *http.Request) {
	setCommonHeaders(request)
	request.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	request.Header.Set("Origin", "https://partner.rta-os.com")
	request.Header.Set("Referer", "https://partner.rta-os.com/")
}
