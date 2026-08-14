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
	dateText := flag.String("date", "", "date to fill in YYYY-MM-DD format")
	sheet := flag.String("sheet", xlsxfill.DefaultSheetName, "worksheet name")
	write := flag.Bool("write", false, "save a new workbook; the default is dry-run")
	overwrite := flag.Bool("overwrite", false, "replace existing L/AB values that differ")
	allowPartial := flag.Bool("allow-partial", false, "write safe rows even when other rows have issues")
	maxQueries := flag.Int("max-queries", 25, "maximum unique store queries for this run")
	onlyRow := flag.Int("row", 0, "only process one worksheet data row; zero processes the date")
	pageConcurrency := flag.Int("page-concurrency", 1, "maximum concurrent RTA page requests after page one")
	loginAttempts := flag.Int("login-attempts", 4, "maximum fresh captcha/login attempts")
	timeout := flag.Duration("timeout", 5*time.Minute, "whole-operation timeout")
	flag.Parse()

	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*dateText), time.Local)
	if err != nil {
		return errors.New("-date must use YYYY-MM-DD")
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
	businessStoreID := strings.TrimSpace(os.Getenv("RTA_BUSINESS_STORE_ID"))
	if account == "" || password == "" || businessStoreID == "" {
		return errors.New("RTA_ACCOUNT, RTA_PASSWORD, and RTA_BUSINESS_STORE_ID are required in the environment or ignored .env file")
	}
	cookieFile := strings.TrimSpace(os.Getenv("RTA_COOKIE_FILE"))
	if cookieFile == "" {
		cookieFile = ".rta-sales.cookies.json"
	}
	client, err := rtasales.NewClient(rtasales.Config{
		Account:         account,
		Password:        password,
		BusinessStoreID: businessStoreID,
		CaptchaSolvers:  []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		CookieFile:      cookieFile,
		PageConcurrency: *pageConcurrency,
		LoginAttempts:   *loginAttempts,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, fillErr := xlsxfill.Fill(ctx, client, xlsxfill.Request{
		InputPath:    *input,
		OutputPath:   *output,
		SheetName:    *sheet,
		Date:         date,
		Mapper:       mapping,
		Write:        *write,
		Overwrite:    *overwrite,
		AllowPartial: *allowPartial,
		MaxQueries:   *maxQueries,
		OnlyRow:      *onlyRow,
	})
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode fill report: %w", err)
	}
	return fillErr
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
