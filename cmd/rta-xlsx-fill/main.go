package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	input := flag.String("input", "", "source .xlsx workbook")
	output := flag.String("output", "", "new output .xlsx workbook (required with -write)")
	mappingPath := flag.String("mapping", "", "optional private .json or .csv store mapping")
	dateText := flag.String("date", "", "single date in YYYY-MM-DD format (mutually exclusive with -from/-to)")
	fromText := flag.String("from", "", "inclusive first date in YYYY-MM-DD format")
	toText := flag.String("to", "", "inclusive last date in YYYY-MM-DD format")
	sheet := flag.String("sheet", xlsxfill.DefaultSheetName, "worksheet name")
	write := flag.Bool("write", false, "save a new workbook; the default is dry-run")
	overwrite := flag.Bool("overwrite", false, "replace existing L/AB values that differ")
	allowPartial := flag.Bool("allow-partial", false, "write safe rows even when other rows have issues")
	maxJobs := flag.Int("max-jobs", xlsxfill.DefaultMaxJobs, "maximum unique date/store jobs for this run")
	maxQueries := flag.Int("max-queries", 0, "deprecated alias for -max-jobs")
	onlyRow := flag.Int("row", 0, "optional diagnostic row; zero scans all rows in the date range")
	concurrency := flag.Int("concurrency", xlsxfill.DefaultConcurrency, "concurrent date/store query jobs (1-4)")
	pageConcurrency := flag.Int("page-concurrency", 1, "maximum concurrent RTA page requests after page one")
	loginAttempts := flag.Int("login-attempts", 4, "maximum fresh captcha/login attempts")
	timeout := flag.Duration("timeout", 0, "optional whole-operation timeout; zero disables it")
	flag.Parse()

	if strings.TrimSpace(*input) == "" {
		return errors.New("-input is required")
	}
	if *write && strings.TrimSpace(*output) == "" {
		return errors.New("-output is required with -write")
	}
	if *write {
		if err := xlsxfill.ValidateOutputPath(*input, *output); err != nil {
			return err
		}
	}
	if *write && *onlyRow > 0 {
		return errors.New("-row is diagnostic-only and cannot be combined with -write")
	}
	if *onlyRow < 0 || *onlyRow == 1 {
		return errors.New("-row must be zero or a data row greater than one")
	}
	if *timeout < 0 {
		return errors.New("-timeout must not be negative")
	}
	from, to, err := commandDateRange(*dateText, *fromText, *toText)
	if err != nil {
		return err
	}
	if *maxQueries != 0 {
		if *maxJobs != xlsxfill.DefaultMaxJobs && *maxJobs != *maxQueries {
			return errors.New("-max-jobs and -max-queries must not conflict")
		}
		*maxJobs = *maxQueries
	}
	var mapping xlsxfill.StoreMapper = xlsxfill.IdentityStoreMap{}
	if strings.TrimSpace(*mappingPath) != "" {
		mapping, err = xlsxfill.LoadStoreMap(*mappingPath)
		if err != nil {
			return err
		}
	}
	account := strings.TrimSpace(os.Getenv("RTA_ACCOUNT"))
	password := os.Getenv("RTA_PASSWORD")
	if account == "" || password == "" {
		return errors.New("RTA_ACCOUNT and RTA_PASSWORD are required in the environment or ignored .env file")
	}
	cookieFile := strings.TrimSpace(os.Getenv("RTA_COOKIE_FILE"))
	if cookieFile == "" {
		cookieFile = ".rta-sales.cookies.json"
	}
	client, err := rtasales.NewClient(rtasales.Config{
		Account:         account,
		Password:        password,
		CaptchaSolvers:  []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		CookieFile:      cookieFile,
		PageConcurrency: *pageConcurrency,
		LoginAttempts:   *loginAttempts,
	})
	if err != nil {
		return err
	}
	ctx := context.Background()
	cancel := func() {}
	if *timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, *timeout)
	}
	defer cancel()
	stores, err := client.Stores(ctx)
	if err != nil {
		return fmt.Errorf("load authorized stores: %w", err)
	}
	allowedStoreIDs := make([]string, len(stores))
	for index, store := range stores {
		allowedStoreIDs[index] = store.BusinessID
	}
	plan, analyzeErr := xlsxfill.Analyze(ctx, client, xlsxfill.BatchRequest{
		InputPath:               *input,
		SheetName:               *sheet,
		From:                    from,
		To:                      to,
		Mapper:                  mapping,
		AllowedBusinessStoreIDs: allowedStoreIDs,
		Overwrite:               *overwrite,
		MaxJobs:                 *maxJobs,
		OnlyRow:                 *onlyRow,
		Concurrency:             *concurrency,
	})
	report := plan.Report
	operationErr := analyzeErr
	if analyzeErr == nil {
		if *write {
			report, operationErr = xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
				OutputPath: *output, AllowPartial: *allowPartial, ForceRecalculate: true,
			})
		} else if len(report.Issues) > 0 && !*allowPartial {
			operationErr = &xlsxfill.ValidationError{IssueCount: len(report.Issues)}
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode fill report: %w", err)
	}
	return operationErr
}

func commandDateRange(dateText, fromText, toText string) (time.Time, time.Time, error) {
	dateText = strings.TrimSpace(dateText)
	fromText = strings.TrimSpace(fromText)
	toText = strings.TrimSpace(toText)
	if dateText != "" {
		if fromText != "" || toText != "" {
			return time.Time{}, time.Time{}, errors.New("-date is mutually exclusive with -from/-to")
		}
		date, err := time.ParseInLocation("2006-01-02", dateText, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("-date must use YYYY-MM-DD")
		}
		return date, date, nil
	}
	if fromText == "" || toText == "" {
		return time.Time{}, time.Time{}, errors.New("use either -date or both -from and -to")
	}
	from, err := time.ParseInLocation("2006-01-02", fromText, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("-from must use YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", toText, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("-to must use YYYY-MM-DD")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("-to must not precede -from")
	}
	return from, to, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open local environment file: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" {
			return fmt.Errorf("invalid local environment file line %d", lineNumber)
		}
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			if value[0] == '"' {
				unquoted, unquoteErr := strconv.Unquote(value)
				if unquoteErr != nil {
					return fmt.Errorf("invalid quoted value on local environment file line %d", lineNumber)
				}
				value = unquoted
			} else {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set local environment value: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read local environment file: %w", err)
	}
	return nil
}
