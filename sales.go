package rtasales

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const salesPageSize = 1000

const trendTransactionPageSize = 50

var trendTransactionColumns = []string{
	"show_date",
	"base_currency",
	"group_sales_ticket_num",
	"group_sales_sales_qty",
	"group_sales_order_sale_untaxed_amt",
	"group_return_return_ticket_num",
	"group_return_refund_sale_num",
	"group_return_refund_sale_untaxed_amt",
	"gross_sales_gross_sale_qty",
	"gross_sales_gross_sale_untaxed_amt",
}

// SalesQuery selects an inclusive calendar-date range for one business store.
// ItemCodes is optional; an empty slice queries all products.
type SalesQuery struct {
	BusinessStoreID string
	StartDate       time.Time
	EndDate         time.Time
	Category        string
	ItemCodes       []string
	// SkipTrend avoids the additional whole-store Trend View request when the
	// caller only needs Article View rows and category metrics.
	SkipTrend bool
}

// SaleItem preserves the typed RTA sales fields and the complete raw row.
// Quantities are float64 so weighted products are not truncated.
type SaleItem struct {
	PurchaseCategory1Name       string         `json:"purchase_category1_name"`
	PurchaseCategory1Code       string         `json:"purchase_category1_code,omitempty"`
	PurchaseCategory2Name       string         `json:"purchase_category2_name"`
	PurchaseCategory2Code       string         `json:"purchase_category2_code,omitempty"`
	PurchaseCategory3Name       string         `json:"purchase_category3_name"`
	PurchaseCategory3Code       string         `json:"purchase_category3_code,omitempty"`
	PurchaseCategory4Name       string         `json:"purchase_category4_name"`
	PurchaseCategory4Code       string         `json:"purchase_category4_code,omitempty"`
	PurchaseCategory5Name       string         `json:"purchase_category5_name"`
	PurchaseCategory5Code       string         `json:"purchase_category5_code,omitempty"`
	Matnr                       string         `json:"matnr"`
	ArticleName                 string         `json:"article_name"`
	BrandName                   string         `json:"brand_name,omitempty"`
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
	// TrendGrossSaleAmount is the whole-store Trend View gross sales amount
	// (sales less returns) for the selected calendar-date range. It is kept
	// separate from the item-filterable Article View TotalAmount.
	TrendGrossSaleAmount *float64 `json:"trend_gross_sale_amount,omitempty"`
	// TotalTransactionCount is the Trend View transaction total for the
	// selected calendar-date range. It is never derived from item rows because
	// one transaction can contain multiple items.
	TotalTransactionCount *float64            `json:"total_transaction_count,omitempty"`
	GrossQuantity         float64             `json:"gross_quantity"`
	Items                 []SaleItem          `json:"items"`
	Categories            []CategoryAggregate `json:"categories"`
	QueryDuration         time.Duration       `json:"query_duration"`
}

type salesQueryPayload struct {
	ViewCode               string `json:"viewCode"`
	StoreClusterCode       string `json:"store_cluster_code"`
	StoreClusterCodeString string `json:"store_cluster_code_str"`
	StoreID                string `json:"store_id"`
	StoreIDString          string `json:"store_id_str"`
	LargeAreaCode          string `json:"large_area_code"`
	LargeAreaCodeString    string `json:"large_area_code_str"`
	TradeFlag              string `json:"trade_flag"`
	TradeFlagString        string `json:"trade_flag_str"`
	Category1Code          string `json:"purchase_category1_code"`
	Category1CodeString    string `json:"purchase_category1_code_str"`
	Category2Code          string `json:"purchase_category2_code"`
	Category2CodeString    string `json:"purchase_category2_code_str"`
	Category3Code          string `json:"purchase_category3_code"`
	Category3CodeString    string `json:"purchase_category3_code_str"`
	Category4Code          string `json:"purchase_category4_code"`
	Category4CodeString    string `json:"purchase_category4_code_str"`
	Category5Code          string `json:"purchase_category5_code"`
	Category5CodeString    string `json:"purchase_category5_code_str"`
	DateType               int    `json:"dateType"`
	Matnr                  string `json:"matnr,omitempty"`
	MatnrString            string `json:"matnr_str,omitempty"`
	TimeQuickType          int    `json:"timeQuickType"`
	DSACurrencyEnable      string `json:"dsaCurrencyEnable"`
	CurrentStartDay        string `json:"currentStartDay"`
	CurrentEndDay          string `json:"currentEndDay"`
	CurrentDateRangeStr    string `json:"currentDateRangeStr"`
	CurrentStart           string `json:"currentStart"`
	CurrentEnd             string `json:"currentEnd"`
	CurrentDateRange       string `json:"currentDateRange"`
	CompareDateRange       string `json:"compareDateRange"`
}

type salesDataEnvelope struct {
	CountResult struct {
		Result []map[string]any `json:"result"`
	} `json:"countResult"`
	ExecuteResult struct {
		Result []map[string]any `json:"result"`
	} `json:"executeResult"`
}

type trendTransactionQueryPayload struct {
	SiteTreeCode1  string `json:"site_tree_code_1"`
	SiteTreeCode2  string `json:"site_tree_code_2"`
	SiteTreeCode3  string `json:"site_tree_code_3"`
	SiteTreeCode4  string `json:"site_tree_code_4"`
	SiteTreeCode5  string `json:"site_tree_code_5"`
	SiteCode       string `json:"site_code"`
	DivisionCode   string `json:"division_code"`
	DepartmentCode string `json:"department_code"`
	CategoryCode   string `json:"category_code"`
	SubCategory    string `json:"sub_category_code"`
	ClassCode      string `json:"class_code"`
	StoreCluster   string `json:"store_cluster"`
	Transaction    string `json:"trans_type"`
	ArticleCode    string `json:"article_code"`
	OrderStatus    string `json:"sale_order_status"`
	DateType       string `json:"dateType"`
	CurrentStart   string `json:"currentStart"`
	CurrentEnd     string `json:"currentEnd"`
	CalendarUnit   string `json:"unit"`
}

type trendTransactionDataEnvelope struct {
	Count int              `json:"count"`
	Data  []map[string]any `json:"data"`
}

type trendTotals struct {
	grossSaleAmount  *float64
	transactionCount *float64
}

// Sales resolves the requested business store through RTA's authenticated
// authorized-store list, fetches every result page, and returns raw rows plus
// deterministic aggregates. RTA's query-only store values remain private.
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
	payload := newSalesPayload(query)
	payload.StoreID = store.upstreamID
	payload.StoreIDString = store.filterText
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
	trend := trendTotals{}
	if !query.SkipTrend {
		trend, err = c.fetchTrendTotals(ctx, query, store)
		if err != nil {
			return nil, err
		}
	}
	total, quantity, categories := aggregateSales(items)
	return &SalesResult{
		Store:                 store.Store,
		StartDate:             query.StartDate.Format("2006-01-02"),
		EndDate:               query.EndDate.Format("2006-01-02"),
		Category:              query.Category,
		ItemCodes:             append([]string(nil), query.ItemCodes...),
		TotalAmount:           total,
		TrendGrossSaleAmount:  trend.grossSaleAmount,
		TotalTransactionCount: trend.transactionCount,
		GrossQuantity:         quantity,
		Items:                 items,
		Categories:            categories,
		QueryDuration:         time.Since(started),
	}, nil
}

func (c *Client) fetchTrendTotals(ctx context.Context, query SalesQuery, store storeRecord) (trendTotals, error) {
	payload := trendTransactionQueryPayload{
		SiteCode:     store.BusinessID,
		DateType:     "1",
		CurrentStart: query.StartDate.Format("2006-01-02"),
		CurrentEnd:   query.EndDate.Format("2006-01-02"),
		CalendarUnit: "one",
	}
	queryJSON, err := json.Marshal(payload)
	if err != nil {
		return trendTotals{}, err
	}
	columnsJSON, err := json.Marshal(trendTransactionColumns)
	if err != nil {
		return trendTotals{}, err
	}
	transactionTotal := 0.0
	grossSaleTotal := 0.0
	found := false
	startKey := query.StartDate.Format("20060102")
	endKey := query.EndDate.Format("20060102")
	for page := 1; ; page++ {
		rows, count, err := c.fetchTrendTransactionPage(ctx, queryJSON, columnsJSON, page)
		if err != nil {
			return trendTotals{}, err
		}
		for _, row := range rows {
			dateText := strings.TrimSpace(stringFrom(row["show_date"]))
			if dateText == "" {
				continue
			}
			dateKey, err := trendDateKey(dateText)
			if err != nil {
				return trendTotals{}, &ProtocolError{Operation: "fetch Trend View totals", Message: "a Trend View row has an invalid date", Err: err}
			}
			if dateKey < startKey || dateKey > endKey {
				continue
			}
			transactionCount := optionalFloatFrom(row["group_sales_ticket_num"])
			if transactionCount == nil {
				return trendTotals{}, &ProtocolError{Operation: "fetch Trend View totals", Message: "a dated Trend View row has no valid transaction count"}
			}
			grossSaleAmount := optionalFloatFrom(row["gross_sales_gross_sale_untaxed_amt"])
			if grossSaleAmount == nil {
				return trendTotals{}, &ProtocolError{Operation: "fetch Trend View totals", Message: "a dated Trend View row has no valid gross sales amount"}
			}
			transactionTotal += *transactionCount
			grossSaleTotal += *grossSaleAmount
			found = true
		}
		if page*trendTransactionPageSize >= count || len(rows) == 0 {
			break
		}
	}
	if !found {
		return trendTotals{}, nil
	}
	return trendTotals{grossSaleAmount: &grossSaleTotal, transactionCount: &transactionTotal}, nil
}

func trendDateKey(value string) (string, error) {
	for _, layout := range []string{"02-01-2006", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("20060102"), nil
		}
	}
	return "", fmt.Errorf("unsupported date %q", value)
}

func (c *Client) fetchTrendTransactionPage(ctx context.Context, queryJSON, columnsJSON []byte, page int) ([]map[string]any, int, error) {
	const operation = "fetch Trend View totals"
	form := url.Values{
		"pageCode":    {"storeRealTimeSalesMannings"},
		"moduleCode":  {"trendTable"},
		"tabCode":     {"trend"},
		"serviceCode": {"achievement"},
		"dataCode":    {"storeRealTimeSalesMannings.trendTable"},
		"queryParam":  {string(queryJSON)},
		"filterParam": {"{}"},
		"showColumns": {string(columnsJSON)},
		"pageNum":     {strconv.Itoa(page)},
		"pageSize":    {strconv.Itoa(trendTransactionPageSize)},
	}
	body, err := c.doAuthenticated(ctx, operation, func(ctx context.Context) (*http.Request, error) {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.cockpit+"/data/pc/v1/query", strings.NewReader(form.Encode()))
		if requestErr == nil {
			setCommonHeaders(request)
			request.Header.Set("Accept", "application/json")
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", "https://partner.rta-os.com")
			request.Header.Set("Referer", "https://partner.rta-os.com/")
		}
		return request, requestErr
	})
	if err != nil {
		return nil, 0, err
	}
	envelope, err := decodeSuccessfulEnvelope(body, operation)
	if err != nil {
		return nil, 0, err
	}
	var data trendTransactionDataEnvelope
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, 0, &ProtocolError{Operation: operation, Message: "invalid Trend View data", Err: err}
	}
	return data.Data, data.Count, nil
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

func newSalesPayload(query SalesQuery) salesQueryPayload {
	start := query.StartDate.Format("20060102")
	end := query.EndDate.Format("20060102")
	codes := strings.Join(query.ItemCodes, ",")
	return salesQueryPayload{
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
			request.Header.Set("Accept-Language", "zh-HK,zh;q=0.9,en;q=0.8")
			request.Header.Set("Origin", "https://partner.rta-os.com")
			request.Header.Set("Referer", "https://partner.rta-os.com/index/partner/index")
			request.Header.Set("Connection", "keep-alive")
			request.Header.Set("Sec-Fetch-Dest", "empty")
			request.Header.Set("Sec-Fetch-Mode", "cors")
			request.Header.Set("Sec-Fetch-Site", "same-site")
			request.Header.Set("Sec-GPC", "1")
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
		counts := data.CountResult.Result[0]
		count := floatFrom(counts["counts"])
		if count == 0 {
			count = floatFrom(counts["_counts"])
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
		"showColumns":    {"{\"purchase_category1_name\":true,\"column_purchase_category1_code\":true,\"purchase_category2_name\":true,\"column_purchase_category2_code\":true,\"purchase_category3_name\":true,\"column_purchase_category3_code\":true,\"purchase_category4_name\":true,\"column_purchase_category4_code\":true,\"purchase_category5_name\":true,\"column_purchase_category5_code\":true,\"matnr\":true,\"article_name\":true,\"brand_name\":true,\"tp_transaction_count\":true,\"tp_transaction_count_agg\":true,\"tp_sale_qty\":true,\"tp_sale_amount\":true,\"tmp2_tp_sale_amount\":true,\"actual_sale_amount_contribution\":false,\"tp_return_transaction_count\":true,\"tp_return_transaction_count_agg\":true,\"tp_return_sale_qty\":true,\"tp_return_sale_amount\":true,\"tmp2_return_sale_amount\":true,\"return_sale_amount_contribution\":false,\"tp_gross_sale_qty\":true,\"tp_gross_sale_amount\":true,\"tmp2_gross_sale_amount\":true,\"gross_sale_amount_contribution\":false}"},
		"filterParam":    {"{}"},
		"orderByColumns": {"{\"tp_sale_amount\":2}"},
		"viewCode":       {"318f39ba93894fb5b85344c24a352201"},
		"pageSize":       {strconv.Itoa(salesPageSize)},
		"columnSeq":      {"purchase_category1_name,column_purchase_category1_code,purchase_category2_name,column_purchase_category2_code,purchase_category3_name,column_purchase_category3_code,purchase_category4_name,column_purchase_category4_code,purchase_category5_name,column_purchase_category5_code,matnr,article_name,brand_name,tp_transaction_count,tp_transaction_count_agg,tp_sale_qty,tp_sale_amount,tmp2_tp_sale_amount,tp_return_transaction_count,tp_return_transaction_count_agg,tp_return_sale_qty,tp_return_sale_amount,tmp2_return_sale_amount,tp_gross_sale_qty,tp_gross_sale_amount,tmp2_gross_sale_amount"},
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
		PurchaseCategory1Code:       firstStringFrom(row, "purchase_category1_code", "column_purchase_category1_code"),
		PurchaseCategory2Name:       stringFrom(row["purchase_category2_name"]),
		PurchaseCategory2Code:       firstStringFrom(row, "purchase_category2_code", "column_purchase_category2_code"),
		PurchaseCategory3Name:       stringFrom(row["purchase_category3_name"]),
		PurchaseCategory3Code:       firstStringFrom(row, "purchase_category3_code", "column_purchase_category3_code"),
		PurchaseCategory4Name:       stringFrom(row["purchase_category4_name"]),
		PurchaseCategory4Code:       firstStringFrom(row, "purchase_category4_code", "column_purchase_category4_code"),
		PurchaseCategory5Name:       stringFrom(row["purchase_category5_name"]),
		PurchaseCategory5Code:       firstStringFrom(row, "purchase_category5_code", "column_purchase_category5_code"),
		Matnr:                       stringFrom(row["matnr"]),
		ArticleName:                 stringFrom(row["article_name"]),
		BrandName:                   stringFrom(row["brand_name"]),
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

func firstStringFrom(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFrom(row[key])); value != "" {
			return value
		}
	}
	return ""
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
