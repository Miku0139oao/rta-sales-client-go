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

// salesMaxInclusiveDays is RTA's inclusive calendar-date limit. Wider ranges
// must be split into multiple requests and merged locally.
const salesMaxInclusiveDays = 90

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
// ItemCodes is optional; an empty slice queries all products. Ranges wider
// than 90 calendar days are split into multiple RTA requests and merged.
type SalesQuery struct {
	BusinessStoreID string
	StartDate       time.Time
	EndDate         time.Time
	Category        string
	ItemCodes       []string
	// SkipTrend avoids the additional whole-store Trend View request when the
	// caller only needs Article View rows and category metrics.
	SkipTrend bool
	// SkipTrendLookback uses the queried dates only for Trend View, instead of
	// also fetching the previous ISO week used by weekly comparison pages.
	SkipTrendLookback bool
	// Compact skips the unused raw-row map and per-category item copies so
	// multi-store desktop analysis does not keep three copies of every SKU.
	Compact bool
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
	TotalTransactionCount *float64 `json:"total_transaction_count,omitempty"`
	// TrendDays is the dated Trend View series. It includes a prior full week
	// so callers can compute this-week vs last-week without another request.
	// Totals above still cover only StartDate through EndDate.
	TrendDays     []TrendDay          `json:"trend_days,omitempty"`
	GrossQuantity float64             `json:"gross_quantity"`
	Items         []SaleItem          `json:"items"`
	Categories    []CategoryAggregate `json:"categories"`
	QueryDuration time.Duration       `json:"query_duration"`
}

// TrendDay is one calendar day's whole-store Trend View totals.
type TrendDay struct {
	Date              string  `json:"date"`
	GrossSaleAmount   float64 `json:"gross_sale_amount"`
	TransactionCount  float64 `json:"transaction_count"`
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
	days             []TrendDay
}

type salesDateWindow struct {
	start time.Time
	end   time.Time
}

// Sales resolves the requested business store through RTA's authenticated
// authorized-store list, fetches every result page, and returns raw rows plus
// deterministic aggregates. RTA's query-only store values remain private.
// Inclusive ranges longer than 90 calendar days are queried as adjacent
// windows and merged so callers can request a wider span than RTA accepts.
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
	items, trend, err := c.fetchSalesWindows(ctx, query, store)
	if err != nil {
		return nil, err
	}
	total, quantity, categories := aggregateSales(items, query.Compact)
	return &SalesResult{
		Store:                 store.Store,
		StartDate:             query.StartDate.Format("2006-01-02"),
		EndDate:               query.EndDate.Format("2006-01-02"),
		Category:              query.Category,
		ItemCodes:             append([]string(nil), query.ItemCodes...),
		TotalAmount:           total,
		TrendGrossSaleAmount:  trend.grossSaleAmount,
		TotalTransactionCount: trend.transactionCount,
		TrendDays:             trend.days,
		GrossQuantity:         quantity,
		Items:                 items,
		Categories:            categories,
		QueryDuration:         time.Since(started),
	}, nil
}

func (c *Client) fetchSalesWindows(ctx context.Context, query SalesQuery, store storeRecord) ([]SaleItem, trendTotals, error) {
	windows := splitSalesDateRange(query.StartDate, query.EndDate)
	type trendFetch struct {
		trend trendTotals
		err   error
	}
	var trendDone chan trendFetch
	if !query.SkipTrend {
		trendDone = make(chan trendFetch, 1)
		go func() {
			var trend trendTotals
			var err error
			if query.SkipTrendLookback {
				trend, err = c.fetchTrendTotals(ctx, query, store)
			} else {
				trend, err = c.fetchTrendSeries(ctx, query, store)
			}
			trendDone <- trendFetch{trend: trend, err: err}
		}()
	}
	var items []SaleItem
	for _, window := range windows {
		windowQuery := query
		windowQuery.StartDate = window.start
		windowQuery.EndDate = window.end
		payload := newSalesPayload(windowQuery)
		payload.StoreID = store.upstreamID
		payload.StoreIDString = store.filterText
		windowItems, err := c.fetchArticleItems(ctx, payload, query.Compact)
		if err != nil {
			if trendDone != nil {
				<-trendDone
			}
			return nil, trendTotals{}, err
		}
		items = append(items, windowItems...)
	}
	if len(windows) > 1 {
		items = mergeSaleItems(items)
	}
	if trendDone == nil {
		return items, trendTotals{}, nil
	}
	fetched := <-trendDone
	if fetched.err != nil {
		return nil, trendTotals{}, fetched.err
	}
	return items, fetched.trend, nil
}

func (c *Client) fetchSalesViews(ctx context.Context, query SalesQuery, payload salesQueryPayload, store storeRecord) ([]SaleItem, trendTotals, error) {
	if query.SkipTrend {
		items, err := c.fetchArticleItems(ctx, payload, query.Compact)
		return items, trendTotals{}, err
	}
	type articleFetch struct {
		items []SaleItem
		err   error
	}
	articles := make(chan articleFetch, 1)
	go func() {
		items, err := c.fetchArticleItems(ctx, payload, query.Compact)
		articles <- articleFetch{items: items, err: err}
	}()
	trend, trendErr := c.fetchTrendTotals(ctx, query, store)
	article := <-articles
	if article.err != nil {
		return nil, trendTotals{}, article.err
	}
	if trendErr != nil {
		return nil, trendTotals{}, trendErr
	}
	return article.items, trend, nil
}

func (c *Client) fetchArticleItems(ctx context.Context, payload salesQueryPayload, compact bool) ([]SaleItem, error) {
	firstItems, totalPages, err := c.fetchSalesPage(ctx, payload, 1, compact)
	if err != nil {
		return nil, err
	}
	pages := make([][]SaleItem, totalPages)
	pages[0] = firstItems
	if totalPages > 1 {
		if err := c.fetchRemainingSalesPages(ctx, payload, pages, compact); err != nil {
			return nil, err
		}
	}
	items := make([]SaleItem, 0)
	for _, page := range pages {
		items = append(items, page...)
	}
	return items, nil
}

func (c *Client) fetchTrendSeries(ctx context.Context, query SalesQuery, store storeRecord) (trendTotals, error) {
	originStart := calendarDate(query.StartDate)
	originEnd := calendarDate(query.EndDate)
	lookbackStart := startOfISOWeek(originStart.AddDate(0, 0, -7))
	var days []TrendDay
	for _, window := range splitSalesDateRange(lookbackStart, originEnd) {
		windowDays, err := c.fetchTrendDays(ctx, window.start, window.end, store)
		if err != nil {
			return trendTotals{}, err
		}
		days = append(days, windowDays...)
	}
	days = mergeTrendDays(days)
	originStartKey := originStart.Format("2006-01-02")
	originEndKey := originEnd.Format("2006-01-02")
	salesTotal := 0.0
	ticketTotal := 0.0
	found := false
	for _, day := range days {
		if day.Date < originStartKey || day.Date > originEndKey {
			continue
		}
		salesTotal += day.GrossSaleAmount
		ticketTotal += day.TransactionCount
		found = true
	}
	if len(days) == 0 {
		return trendTotals{}, nil
	}
	result := trendTotals{days: days}
	if found {
		result.grossSaleAmount = &salesTotal
		result.transactionCount = &ticketTotal
	}
	return result, nil
}

func (c *Client) fetchTrendTotals(ctx context.Context, query SalesQuery, store storeRecord) (trendTotals, error) {
	days, err := c.fetchTrendDays(ctx, query.StartDate, query.EndDate, store)
	if err != nil {
		return trendTotals{}, err
	}
	if len(days) == 0 {
		return trendTotals{}, nil
	}
	salesTotal := 0.0
	ticketTotal := 0.0
	for _, day := range days {
		salesTotal += day.GrossSaleAmount
		ticketTotal += day.TransactionCount
	}
	return trendTotals{grossSaleAmount: &salesTotal, transactionCount: &ticketTotal, days: days}, nil
}

func (c *Client) fetchTrendDays(ctx context.Context, start, end time.Time, store storeRecord) ([]TrendDay, error) {
	payload := trendTransactionQueryPayload{
		SiteCode:     store.BusinessID,
		DateType:     "1",
		CurrentStart: start.Format("2006-01-02"),
		CurrentEnd:   end.Format("2006-01-02"),
		CalendarUnit: "one",
	}
	queryJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	columnsJSON, err := json.Marshal(trendTransactionColumns)
	if err != nil {
		return nil, err
	}
	startKey := calendarDate(start).Format("20060102")
	endKey := calendarDate(end).Format("20060102")
	days := make([]TrendDay, 0)
	for page := 1; ; page++ {
		rows, count, err := c.fetchTrendTransactionPage(ctx, queryJSON, columnsJSON, page)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			dateText := strings.TrimSpace(stringFrom(row["show_date"]))
			if dateText == "" {
				continue
			}
			parsed, err := parseTrendDate(dateText)
			if err != nil {
				return nil, &ProtocolError{Operation: "fetch Trend View totals", Message: "a Trend View row has an invalid date", Err: err}
			}
			dateKey := parsed.Format("20060102")
			if dateKey < startKey || dateKey > endKey {
				continue
			}
			transactionCount := optionalFloatFrom(row["group_sales_ticket_num"])
			if transactionCount == nil {
				return nil, &ProtocolError{Operation: "fetch Trend View totals", Message: "a dated Trend View row has no valid transaction count"}
			}
			grossSaleAmount := optionalFloatFrom(row["gross_sales_gross_sale_untaxed_amt"])
			if grossSaleAmount == nil {
				return nil, &ProtocolError{Operation: "fetch Trend View totals", Message: "a dated Trend View row has no valid gross sales amount"}
			}
			days = append(days, TrendDay{
				Date:             parsed.Format("2006-01-02"),
				GrossSaleAmount:  *grossSaleAmount,
				TransactionCount: *transactionCount,
			})
		}
		if page*trendTransactionPageSize >= count || len(rows) == 0 {
			break
		}
	}
	return days, nil
}

func trendDateKey(value string) (string, error) {
	parsed, err := parseTrendDate(value)
	if err != nil {
		return "", err
	}
	return parsed.Format("20060102"), nil
}

func parseTrendDate(value string) (time.Time, error) {
	for _, layout := range []string{"02-01-2006", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date %q", value)
}

func startOfISOWeek(value time.Time) time.Time {
	value = calendarDate(value)
	weekday := int(value.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return value.AddDate(0, 0, -(weekday - 1))
}

func mergeTrendDays(days []TrendDay) []TrendDay {
	if len(days) < 2 {
		return days
	}
	indexByDate := make(map[string]int, len(days))
	merged := make([]TrendDay, 0, len(days))
	for _, day := range days {
		if index, exists := indexByDate[day.Date]; exists {
			merged[index].GrossSaleAmount += day.GrossSaleAmount
			merged[index].TransactionCount += day.TransactionCount
			continue
		}
		indexByDate[day.Date] = len(merged)
		merged = append(merged, day)
	}
	sort.Slice(merged, func(left, right int) bool { return merged[left].Date < merged[right].Date })
	return merged
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

func splitSalesDateRange(start, end time.Time) []salesDateWindow {
	start = calendarDate(start)
	end = calendarDate(end)
	windows := make([]salesDateWindow, 0, 1)
	for !start.After(end) {
		windowEnd := start.AddDate(0, 0, salesMaxInclusiveDays-1)
		if windowEnd.After(end) {
			windowEnd = end
		}
		windows = append(windows, salesDateWindow{start: start, end: windowEnd})
		start = windowEnd.AddDate(0, 0, 1)
	}
	return windows
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func mergeSaleItems(items []SaleItem) []SaleItem {
	if len(items) < 2 {
		return items
	}
	indexByMatnr := make(map[string]int, len(items))
	merged := make([]SaleItem, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.Matnr)
		if key == "" {
			merged = append(merged, item)
			continue
		}
		if index, exists := indexByMatnr[key]; exists {
			merged[index] = addSaleItems(merged[index], item)
			continue
		}
		indexByMatnr[key] = len(merged)
		merged = append(merged, item)
	}
	return merged
}

func addSaleItems(left, right SaleItem) SaleItem {
	left.TPTransactionCount += right.TPTransactionCount
	left.TPTransactionCountAgg = addOptionalFloats(left.TPTransactionCountAgg, right.TPTransactionCountAgg)
	left.TPSaleQuantity += right.TPSaleQuantity
	left.TPSaleAmount += right.TPSaleAmount
	left.TPReturnTransactionCount += right.TPReturnTransactionCount
	left.TPReturnTransactionCountAgg = addOptionalFloats(left.TPReturnTransactionCountAgg, right.TPReturnTransactionCountAgg)
	left.TPReturnSaleQuantity += right.TPReturnSaleQuantity
	left.TPReturnSaleAmount += right.TPReturnSaleAmount
	left.TPGrossSaleQuantity += right.TPGrossSaleQuantity
	left.TPGrossSaleAmount += right.TPGrossSaleAmount
	if strings.TrimSpace(left.ArticleName) == "" {
		left.ArticleName = right.ArticleName
	}
	if strings.TrimSpace(left.BrandName) == "" {
		left.BrandName = right.BrandName
	}
	return left
}

func addTrendTotals(left, right trendTotals) trendTotals {
	return trendTotals{
		grossSaleAmount:  addOptionalFloats(left.grossSaleAmount, right.grossSaleAmount),
		transactionCount: addOptionalFloats(left.transactionCount, right.transactionCount),
		days:             mergeTrendDays(append(append([]TrendDay(nil), left.days...), right.days...)),
	}
}

func addOptionalFloats(left, right *float64) *float64 {
	if left == nil && right == nil {
		return nil
	}
	sum := 0.0
	if left != nil {
		sum += *left
	}
	if right != nil {
		sum += *right
	}
	return &sum
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

func (c *Client) fetchSalesPage(ctx context.Context, payload salesQueryPayload, page int, compact bool) ([]SaleItem, int, error) {
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
		items = append(items, saleItemFromRow(row, compact))
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

func (c *Client) fetchRemainingSalesPages(ctx context.Context, payload salesQueryPayload, pages [][]SaleItem, compact bool) error {
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
				items, _, err := c.fetchSalesPage(workContext, payload, page, compact)
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

func saleItemFromRow(row map[string]any, compact bool) SaleItem {
	var raw map[string]any
	if !compact {
		raw = make(map[string]any, len(row))
		for key, value := range row {
			raw[key] = value
		}
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

func aggregateSales(items []SaleItem, compact bool) (float64, float64, []CategoryAggregate) {
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
		if !compact {
			aggregate.Items = append(aggregate.Items, item)
		}
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
