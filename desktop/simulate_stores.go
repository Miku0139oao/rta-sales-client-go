package desktop

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

const (
	simulatedStoreMarker  = "~sim"
	maximumSimulateStores = 32
)

// simulatingClient exposes extra store identities for local multi-store tests
// while still querying the account's real authorized stores.
type inflightSales struct {
	done   chan struct{}
	result *rtasales.SalesResult
	err    error
}

type simulatingClient struct {
	inner accountClient
	count int

	mu      sync.Mutex
	cache   map[string]*rtasales.SalesResult
	pending map[string]*inflightSales
}

func maybeSimulateClient(client accountClient, count int) accountClient {
	count = normalizeSimulateStoreCount(count)
	if client == nil || count == 0 {
		return client
	}
	return &simulatingClient{
		inner:   client,
		count:   count,
		cache:   make(map[string]*rtasales.SalesResult),
		pending: make(map[string]*inflightSales),
	}
}

func normalizeSimulateStoreCount(value int) int {
	if value <= 0 {
		return 0
	}
	if value > maximumSimulateStores {
		return maximumSimulateStores
	}
	return value
}

func (c *simulatingClient) Stores(ctx context.Context) ([]rtasales.Store, error) {
	stores, err := c.inner.Stores(ctx)
	if err != nil {
		return nil, err
	}
	return expandSimulatedStores(stores, c.count), nil
}

func (c *simulatingClient) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	sourceID, scale, simulated := resolveSimulatedStore(query.BusinessStoreID)
	query.BusinessStoreID = sourceID
	result, err := c.lookupOrFetch(ctx, query)
	if err != nil {
		return nil, err
	}
	if simulated {
		scaleSalesResult(result, scale)
	}
	return result, nil
}

func (c *simulatingClient) lookupOrFetch(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	key := simulatedSalesCacheKey(query)
	c.mu.Lock()
	if cached, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return cloneSalesResult(cached), nil
	}
	if wait, ok := c.pending[key]; ok {
		c.mu.Unlock()
		select {
		case <-wait.done:
			if wait.err != nil {
				return nil, wait.err
			}
			return cloneSalesResult(wait.result), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	wait := &inflightSales{done: make(chan struct{})}
	c.pending[key] = wait
	c.mu.Unlock()

	result, err := c.inner.Sales(ctx, query)
	c.mu.Lock()
	wait.result = result
	wait.err = err
	if err == nil {
		c.cache[key] = result
	}
	delete(c.pending, key)
	close(wait.done)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return cloneSalesResult(result), nil
}

func expandSimulatedStores(stores []rtasales.Store, count int) []rtasales.Store {
	count = normalizeSimulateStoreCount(count)
	if count == 0 || len(stores) == 0 || len(stores) >= count {
		return stores
	}
	expanded := make([]rtasales.Store, 0, count)
	expanded = append(expanded, stores...)
	for index := len(stores); index < count; index++ {
		source := stores[index%len(stores)]
		serial := index - len(stores) + 2
		expanded = append(expanded, rtasales.Store{
			BusinessID: fmt.Sprintf("%s%s%02d", source.BusinessID, simulatedStoreMarker, serial),
			Label:      fmt.Sprintf("%s · 模擬 %02d", strings.TrimSpace(source.Label), serial),
		})
	}
	return expanded
}

func resolveSimulatedStore(businessID string) (sourceID string, scale float64, simulated bool) {
	businessID = strings.TrimSpace(businessID)
	marker := strings.LastIndex(businessID, simulatedStoreMarker)
	if marker <= 0 {
		return businessID, 1, false
	}
	serial, err := strconv.Atoi(businessID[marker+len(simulatedStoreMarker):])
	if err != nil || serial < 2 || serial > maximumSimulateStores {
		return businessID, 1, false
	}
	return businessID[:marker], simulatedStoreScale(serial), true
}

func simulatedStoreScale(serial int) float64 {
	return 0.55 + float64(serial-1)*0.05
}

func simulatedSalesCacheKey(query rtasales.SalesQuery) string {
	return strings.Join([]string{
		query.BusinessStoreID,
		query.StartDate.Format("2006-01-02"),
		query.EndDate.Format("2006-01-02"),
		strconv.FormatBool(query.SkipTrend),
	}, "|")
}

func cloneSalesResult(source *rtasales.SalesResult) *rtasales.SalesResult {
	if source == nil {
		return nil
	}
	clone := *source
	if source.TrendGrossSaleAmount != nil {
		value := *source.TrendGrossSaleAmount
		clone.TrendGrossSaleAmount = &value
	}
	if source.TotalTransactionCount != nil {
		value := *source.TotalTransactionCount
		clone.TotalTransactionCount = &value
	}
	if source.Items != nil {
		clone.Items = append([]rtasales.SaleItem(nil), source.Items...)
	}
	if source.Categories != nil {
		clone.Categories = make([]rtasales.CategoryAggregate, len(source.Categories))
		copy(clone.Categories, source.Categories)
		for index := range clone.Categories {
			if clone.Categories[index].Items != nil {
				clone.Categories[index].Items = append([]rtasales.SaleItem(nil), clone.Categories[index].Items...)
			}
		}
	}
	if source.ItemCodes != nil {
		clone.ItemCodes = append([]string(nil), source.ItemCodes...)
	}
	if source.TrendDays != nil {
		clone.TrendDays = append([]rtasales.TrendDay(nil), source.TrendDays...)
	}
	return &clone
}

func scaleSalesResult(result *rtasales.SalesResult, scale float64) {
	if result == nil || scale == 1 {
		return
	}
	result.TotalAmount *= scale
	result.GrossQuantity *= scale
	if result.TrendGrossSaleAmount != nil {
		*result.TrendGrossSaleAmount *= scale
	}
	if result.TotalTransactionCount != nil {
		*result.TotalTransactionCount *= scale
	}
	for index := range result.TrendDays {
		result.TrendDays[index].GrossSaleAmount *= scale
		result.TrendDays[index].TransactionCount *= scale
	}
	for index := range result.Items {
		scaleSaleItem(&result.Items[index], scale)
	}
	for index := range result.Categories {
		result.Categories[index].TotalAmount *= scale
		result.Categories[index].GrossQuantity *= scale
		for itemIndex := range result.Categories[index].Items {
			scaleSaleItem(&result.Categories[index].Items[itemIndex], scale)
		}
	}
}

func scaleSaleItem(item *rtasales.SaleItem, scale float64) {
	item.TPTransactionCount *= scale
	item.TPSaleQuantity *= scale
	item.TPSaleAmount *= scale
	item.TPReturnTransactionCount *= scale
	item.TPReturnSaleQuantity *= scale
	item.TPReturnSaleAmount *= scale
	item.TPGrossSaleQuantity *= scale
	item.TPGrossSaleAmount *= scale
	if item.TPTransactionCountAgg != nil {
		value := *item.TPTransactionCountAgg * scale
		item.TPTransactionCountAgg = &value
	}
	if item.TPReturnTransactionCountAgg != nil {
		value := *item.TPReturnTransactionCountAgg * scale
		item.TPReturnTransactionCountAgg = &value
	}
}
