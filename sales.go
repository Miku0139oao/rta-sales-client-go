package rtasales

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const salesPageSize = 1000

// SalesQuery selects an inclusive calendar-date range for one business store.
// ItemCodes is optional; an empty slice queries all products.
type SalesQuery struct {
	BusinessStoreID string
	StartDate       time.Time
	EndDate         time.Time
	Category        string
	ItemCodes       []string
}

// SaleItem preserves the typed RTA sales fields and the complete raw row.
// Quantities are float64 so weighted products are not truncated.
type SaleItem struct {
	PurchaseCategory1Name       string         `json:"purchase_category1_name"`
	PurchaseCategory2Name       string         `json:"purchase_category2_name"`
	PurchaseCategory3Name       string         `json:"purchase_category3_name"`
	PurchaseCategory4Name       string         `json:"purchase_category4_name"`
	PurchaseCategory5Name       string         `json:"purchase_category5_name"`
	Matnr                       string         `json:"matnr"`
	ArticleName                 string         `json:"article_name"`
	TPTransactionCount          float64        `json:"tp_transaction_count"`
	TPTransactionCountAgg       *float64       `json:"tp_transaction_count_agg,omitempty"`
	TPSaleQuantity              float64        `json:"tp_sale_qty"`
	TPSaleAmount                float64        `json:"tp_sale_amount"`
	TPReturnTransactionCount    float64        `json:"tp_return_transaction_count"`
	TPReturnTransactionCountAgg *float64       `json:"tp_return_transaction_count_agg,omitempty"`
	TPReturnSaleQuantity        float64        `json:"tp_return_sale_qty"`
	TPReturnSaleAmount          float64        `json:"tp_return_sale_amount"`
	TPGrossSaleQuantity         float64        `json:"tp_gross_sale_qty"`
	TPGrossSaleAmount           float64        `json:"tp_gross_sale_amount"`
	Raw                         map[string]any `json:"raw"`
}

type CategoryAggregate struct {
	Name          string     `json:"name"`
	TotalAmount   float64    `json:"total_amount"`
	GrossQuantity float64    `json:"gross_quantity"`
	Items         []SaleItem `json:"items"`
}

type SalesResult struct {
	Store       Store    `json:"store"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Category    string   `json:"category"`
	ItemCodes   []string `json:"item_codes,omitempty"`
	TotalAmount float64  `json:"total_amount"`
	// TotalTransactionCount is populated only when RTA returns one consistent
	// aggregate value. It is never derived by summing item-level transaction
	// counts because one transaction may contain multiple items.
	TotalTransactionCount *float64            `json:"total_transaction_count,omitempty"`
	GrossQuantity         float64             `json:"gross_quantity"`
	Items                 []SaleItem          `json:"items"`
	Categories            []CategoryAggregate `json:"categories"`
	QueryDuration         time.Duration       `json:"query_duration"`
}

type salesQueryPayload struct {
	StoreID             string `json:"store_id"`
	StoreIDString       string `json:"store_id_str"`
	DateType            int    `json:"dateType"`
	TimeQuickType       int    `json:"timeQuickType"`
	CurrentStartDay     string `json:"currentStartDay"`
	CurrentEndDay       string `json:"currentEndDay"`
	CurrentDateRangeStr string `json:"currentDateRangeStr"`
	CurrentStart        string `json:"currentStart"`
	CurrentEnd          string `json:"currentEnd"`
	CurrentDateRange    string `json:"currentDateRange"`
	CompareDateRange    string `json:"compareDateRange"`
	Matnr               string `json:"matnr,omitempty"`
	MatnrString         string `json:"matnr_str,omitempty"`
}

type salesDataEnvelope struct {
	CountResult struct {
		Result []map[string]any `json:"result"`
	} `json:"countResult"`
	ExecuteResult struct {
		Result []map[string]any `json:"result"`
	} `json:"executeResult"`
}

// Sales resolves an authorized business store, fetches every RTA result page,
// and returns raw rows plus deterministic aggregates. Any failed page fails
// the whole query.
func (c *Client) Sales(ctx context.Context, query SalesQuery) (*SalesResult, error) {
	started := time.Now()
	query, err := validateSalesQuery(query)
	if err != nil {
		return nil, err
	}
	store, err := c.resolveStore(ctx, query.BusinessStoreID)
	if err != nil {
		return nil, err
	}
	payload := newSalesPayload(store.upstreamID, query)
	firstItems, totalPages, err := c.fetchSalesPage(ctx, payload, 1)
	if err != nil {
		return nil, err
	}
	pages := make([][]SaleItem, totalPages)
	pages[0] = firstItems
	if totalPages > 1 {
		if err := c.fetchRemainingSalesPages(ctx, payload, pages); err != nil {
			return nil, err
		}
	}
	items := make([]SaleItem, 0)
	for _, page := range pages {
		items = append(items, page...)
	}
	total, quantity, categories := aggregateSales(items)
	totalTransactionCount := aggregateTransactionCount(items)
	return &SalesResult{
		Store:                 store.Store,
		StartDate:             query.StartDate.Format("2006-01-02"),
		EndDate:               query.EndDate.Format("2006-01-02"),
		Category:              query.Category,
		ItemCodes:             append([]string(nil), query.ItemCodes...),
		TotalAmount:           total,
		TotalTransactionCount: totalTransactionCount,
		GrossQuantity:         quantity,
		Items:                 items,
		Categories:            categories,
		QueryDuration:         time.Since(started),
	}, nil
}

func validateSalesQuery(query SalesQuery) (SalesQuery, error) {
	query.BusinessStoreID = strings.TrimSpace(query.BusinessStoreID)
	if query.BusinessStoreID == "" {
		return query, &InputError{Field: "BusinessStoreID", Message: "is required"}
	}
	if query.StartDate.IsZero() {
		return query, &InputError{Field: "StartDate", Message: "is required"}
	}
	if query.EndDate.IsZero() {
		return query, &InputError{Field: "EndDate", Message: "is required"}
	}
	startKey := query.StartDate.Format("20060102")
	endKey := query.EndDate.Format("20060102")
	if endKey < startKey {
		return query, &InputError{Field: "EndDate", Message: "must not precede StartDate"}
	}
	query.Category = strings.TrimSpace(query.Category)
	if query.Category == "" {
		query.Category = "全部商品"
	}
	seen := make(map[string]struct{}, len(query.ItemCodes))
	codes := make([]string, 0, len(query.ItemCodes))
	for _, code := range query.ItemCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	query.ItemCodes = codes
	return query, nil
}

func newSalesPayload(upstreamStoreID string, query SalesQuery) salesQueryPayload {
	start := query.StartDate.Format("20060102")
	end := query.EndDate.Format("20060102")
	codes := strings.Join(query.ItemCodes, ",")
	return salesQueryPayload{
		StoreID:             upstreamStoreID,
		StoreIDString:       upstreamStoreID,
		DateType:            1,
		TimeQuickType:       3,
		CurrentStartDay:     start,
		CurrentEndDay:       end,
		CurrentDateRangeStr: query.StartDate.Format("2006.01.02") + "-" + query.EndDate.Format("2006.01.02"),
		CurrentStart:        start,
		CurrentEnd:          end,
		CurrentDateRange:    start + "~" + end,
		CompareDateRange:    "undefined~",
		Matnr:               codes,
		MatnrString:         codes,
	}
}

func (c *Client) fetchSalesPage(ctx context.Context, payload salesQueryPayload, page int) ([]SaleItem, int, error) {
	queryJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	form := fixedSalesForm(page)
	form.Set("queryParam", string(queryJSON))
	body, err := c.doAuthenticated(ctx, fmt.Sprintf("fetch sales page %d", page), func(ctx context.Context) (*http.Request, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.dsa+"/open/data/query", strings.NewReader(form.Encode()))
		if err == nil {
			setCommonHeaders(request)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
			request.Header.Set("Origin", "https://partner.rta-os.com")
			request.Header.Set("Referer", "https://partner.rta-os.com/index/partner/index")
		}
		return request, err
	})
	if err != nil {
		return nil, 0, err
	}
	envelope, err := decodeSuccessfulEnvelope(body, fmt.Sprintf("fetch sales page %d", page))
	if err != nil {
		return nil, 0, err
	}
	var data salesDataEnvelope
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, 0, &ProtocolError{Operation: fmt.Sprintf("fetch sales page %d", page), Message: "invalid sales data", Err: err}
	}
	items := make([]SaleItem, 0, len(data.ExecuteResult.Result))
	for _, row := range data.ExecuteResult.Result {
		items = append(items, saleItemFromRow(row))
	}
	totalPages := 1
	if page == 1 && len(data.CountResult.Result) > 0 {
		count := floatFrom(data.CountResult.Result[0]["counts"])
		if count == 0 {
			count = floatFrom(data.CountResult.Result[0]["_counts"])
		}
		if count > 0 {
			totalPages = (int(count) + salesPageSize - 1) / salesPageSize
		}
	}
	return items, totalPages, nil
}

func (c *Client) fetchRemainingSalesPages(ctx context.Context, payload salesQueryPayload, pages [][]SaleItem) error {
	workContext, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var waitGroup sync.WaitGroup
	var firstError error
	var errorOnce sync.Once
	workers := c.pageConcurrency
	if workers > len(pages)-1 {
		workers = len(pages) - 1
	}
	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for page := range jobs {
				items, _, err := c.fetchSalesPage(workContext, payload, page)
				if err != nil {
					errorOnce.Do(func() {
						firstError = err
						cancel()
					})
					continue
				}
				pages[page-1] = items
			}
		}()
	}
sendPages:
	for page := 2; page <= len(pages); page++ {
		select {
		case jobs <- page:
		case <-workContext.Done():
			break sendPages
		}
	}
	close(jobs)
	waitGroup.Wait()
	return firstError
}

func fixedSalesForm(page int) url.Values {
	return url.Values{
		"showColumns":    {"{\"purchase_category1_name\":true,\"column_purchase_category1_code\":false,\"purchase_category2_name\":true,\"column_purchase_category2_code\":false,\"purchase_category3_name\":true,\"column_purchase_category3_code\":false,\"purchase_category4_name\":true,\"column_purchase_category4_code\":false,\"purchase_category5_name\":true,\"column_purchase_category5_code\":false,\"matnr\":true,\"article_name\":true,\"brand_name\":false,\"tp_transaction_count\":true,\"tp_transaction_count_agg\":true,\"tp_sale_qty\":true,\"tp_sale_amount\":true,\"tmp2_tp_sale_amount\":true,\"actual_sale_amount_contribution\":false,\"tp_return_transaction_count\":true,\"tp_return_transaction_count_agg\":true,\"tp_return_sale_qty\":true,\"tp_return_sale_amount\":true,\"tmp2_return_sale_amount\":true,\"return_sale_amount_contribution\":false,\"tp_gross_sale_qty\":true,\"tp_gross_sale_amount\":true,\"tmp2_gross_sale_amount\":true,\"gross_sale_amount_contribution\":false}"},
		"filterParam":    {"{}"},
		"orderByColumns": {"{\"tp_sale_amount\":2}"},
		"viewCode":       {"318f39ba93894fb5b85344c24a352201"},
		"pageSize":       {strconv.Itoa(salesPageSize)},
		"columnSeq":      {"purchase_category1_name,purchase_category2_name,purchase_category3_name,purchase_category4_name,purchase_category5_name,matnr,article_name,tp_transaction_count,tp_transaction_count_agg,tp_sale_qty,tp_sale_amount,tmp2_tp_sale_amount,tp_return_transaction_count,tp_return_transaction_count_agg,tp_return_sale_qty,tp_return_sale_amount,tmp2_return_sale_amount,tp_gross_sale_qty,tp_gross_sale_amount,tmp2_gross_sale_amount"},
		"pageNum":        {strconv.Itoa(page)},
	}
}

func saleItemFromRow(row map[string]any) SaleItem {
	raw := make(map[string]any, len(row))
	for key, value := range row {
		raw[key] = value
	}
	return SaleItem{
		PurchaseCategory1Name:       stringFrom(row["purchase_category1_name"]),
		PurchaseCategory2Name:       stringFrom(row["purchase_category2_name"]),
		PurchaseCategory3Name:       stringFrom(row["purchase_category3_name"]),
		PurchaseCategory4Name:       stringFrom(row["purchase_category4_name"]),
		PurchaseCategory5Name:       stringFrom(row["purchase_category5_name"]),
		Matnr:                       stringFrom(row["matnr"]),
		ArticleName:                 stringFrom(row["article_name"]),
		TPTransactionCount:          floatFrom(row["tp_transaction_count"]),
		TPTransactionCountAgg:       optionalFloatFrom(row["tp_transaction_count_agg"]),
		TPSaleQuantity:              floatFrom(row["tp_sale_qty"]),
		TPSaleAmount:                floatFrom(row["tp_sale_amount"]),
		TPReturnTransactionCount:    floatFrom(row["tp_return_transaction_count"]),
		TPReturnTransactionCountAgg: optionalFloatFrom(row["tp_return_transaction_count_agg"]),
		TPReturnSaleQuantity:        floatFrom(row["tp_return_sale_qty"]),
		TPReturnSaleAmount:          floatFrom(row["tp_return_sale_amount"]),
		TPGrossSaleQuantity:         floatFrom(row["tp_gross_sale_qty"]),
		TPGrossSaleAmount:           floatFrom(row["tp_gross_sale_amount"]),
		Raw:                         raw,
	}
}

func aggregateTransactionCount(items []SaleItem) *float64 {
	if len(items) == 0 {
		return nil
	}
	var aggregate *float64
	for _, item := range items {
		if item.TPTransactionCountAgg == nil {
			return nil
		}
		value := *item.TPTransactionCountAgg
		if aggregate == nil {
			aggregate = new(float64)
			*aggregate = value
			continue
		}
		tolerance := math.Max(1, math.Max(math.Abs(*aggregate), math.Abs(value))) * 1e-9
		if math.Abs(*aggregate-value) > tolerance {
			return nil
		}
	}
	return aggregate
}

func aggregateSales(items []SaleItem) (float64, float64, []CategoryAggregate) {
	total := 0.0
	quantity := 0.0
	byName := make(map[string]*CategoryAggregate)
	for _, item := range items {
		total += item.TPSaleAmount
		quantity += item.TPGrossSaleQuantity
		name := strings.Trim(strings.TrimSpace(item.PurchaseCategory4Name)+"/"+strings.TrimSpace(item.PurchaseCategory5Name), "/")
		aggregate := byName[name]
		if aggregate == nil {
			aggregate = &CategoryAggregate{Name: name}
			byName[name] = aggregate
		}
		aggregate.TotalAmount += item.TPSaleAmount
		aggregate.GrossQuantity += item.TPGrossSaleQuantity
		aggregate.Items = append(aggregate.Items, item)
	}
	categories := make([]CategoryAggregate, 0, len(byName))
	for _, aggregate := range byName {
		categories = append(categories, *aggregate)
	}
	sort.Slice(categories, func(left, right int) bool {
		if categories[left].TotalAmount == categories[right].TotalAmount {
			return categories[left].Name < categories[right].Name
		}
		return categories[left].TotalAmount > categories[right].TotalAmount
	})
	return total, quantity, categories
}
