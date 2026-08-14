package xlsxfill

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// StoreMap is a private runtime mapping from workbook-facing store codes to
// RTA business store IDs. Callers should not commit populated maps.
type StoreMap map[string]string

func (mapping StoreMap) ResolveStore(workbookStoreID string) (string, bool) {
	value, ok := mapping[strings.TrimSpace(workbookStoreID)]
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

// IdentityStoreMap passes the workbook store code directly to Client.Sales.
// This is appropriate when column C already contains the business-facing ID
// returned by Client.Stores; Client still resolves it through its private,
// authenticated upstream store table.
type IdentityStoreMap struct{}

func (IdentityStoreMap) ResolveStore(workbookStoreID string) (string, bool) {
	value := strings.TrimSpace(workbookStoreID)
	return value, value != ""
}

// LoadStoreMap reads either a JSON object or a CSV file with
// sheet_store_id,rta_business_store_id headers.
func LoadStoreMap(path string) (StoreMap, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open store mapping: %w", err)
	}
	defer func() { _ = file.Close() }()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return loadJSONStoreMap(file)
	case ".csv":
		return loadCSVStoreMap(file)
	default:
		return nil, fmt.Errorf("store mapping must use .json or .csv")
	}
}

func loadJSONStoreMap(reader io.Reader) (StoreMap, error) {
	decoder := json.NewDecoder(reader)
	var raw map[string]string
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode JSON store mapping: %w", err)
	}
	return normalizeStoreMap(raw)
}

func loadCSVStoreMap(reader io.Reader) (StoreMap, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV store mapping header: %w", err)
	}
	indexes := make(map[string]int, len(header))
	for index, name := range header {
		indexes[strings.ToLower(strings.TrimSpace(name))] = index
	}
	left, leftOK := indexes["sheet_store_id"]
	right, rightOK := indexes["rta_business_store_id"]
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("CSV store mapping requires sheet_store_id and rta_business_store_id headers")
	}
	raw := make(map[string]string)
	for line := 2; ; line++ {
		record, readErr := csvReader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read CSV store mapping line %d: %w", line, readErr)
		}
		if left >= len(record) || right >= len(record) {
			return nil, fmt.Errorf("CSV store mapping line %d is incomplete", line)
		}
		key := strings.TrimSpace(record[left])
		value := strings.TrimSpace(record[right])
		if key == "" && value == "" {
			continue
		}
		if _, exists := raw[key]; exists {
			return nil, fmt.Errorf("CSV store mapping line %d duplicates a workbook store ID", line)
		}
		raw[key] = value
	}
	return normalizeStoreMap(raw)
}

func normalizeStoreMap(raw map[string]string) (StoreMap, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("store mapping is empty")
	}
	normalized := make(StoreMap, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("store mapping contains an empty ID")
		}
		if _, exists := normalized[key]; exists {
			return nil, fmt.Errorf("store mapping contains a duplicate workbook store ID")
		}
		normalized[key] = value
	}
	return normalized, nil
}
