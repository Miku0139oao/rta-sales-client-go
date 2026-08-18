package rtasales

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	standardjar "net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	persistentjar "github.com/juju/persistent-cookiejar"
)

const (
	defaultPageConcurrency     = 16
	defaultLoginAttempts       = 4
	maximumLoginAttempts       = 10
	maxResponseBytes           = 64 << 20
	defaultMaxIdleConns        = 1024
	defaultMaxIdleConnsPerHost = 1024
	defaultMaxConnsPerHost     = 1024
	userAgent                  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0.0.0 Safari/537.36 Edg/118.0.1264.123"
)

type endpoints struct {
	sso        string
	dsa        string
	cockpit    string
	authStores string
}

var productionEndpoints = endpoints{
	sso:        "https://mansso.rta-os.com",
	dsa:        "https://dsa-api-partner.rta-os.com",
	cockpit:    "https://cockpit-report-gateway.rta-os.com",
	authStores: "https://manrdprequisition-partner.rta-os.com",
}

// Config configures one RTA account. The client retrieves that account's
// authorized business-store mapping directly from RTA.
type Config struct {
	Account        string
	Password       string
	CaptchaSolvers []CaptchaSolver
	CookieFile     string
	// CookieStore persists the session without exposing cookie bytes to a
	// plaintext file. It is mutually exclusive with CookieFile.
	CookieStore     CookieStore
	HTTPClient      *http.Client
	PageConcurrency int
	// LoginAttempts is the maximum number of fresh captcha/login attempts.
	// Non-positive values use four; values above ten are rejected.
	LoginAttempts int
}

// Client is safe for concurrent use.
type Client struct {
	account         string
	password        string
	captchaSolvers  []CaptchaSolver
	httpClient      *http.Client
	saveCookies     func() error
	pageConcurrency int
	loginAttempts   int
	endpoints       endpoints

	loginMu        sync.Mutex
	sessionVersion atomic.Uint64

	storesMu     sync.RWMutex
	stores       []storeRecord
	storesLoaded bool
	storeLoadMu  sync.Mutex
}

// NewClient creates an account-scoped client. It does not perform network I/O;
// the first Stores or Sales request logs in automatically if saved cookies are
// absent or expired.
func NewClient(config Config) (*Client, error) {
	account := strings.TrimSpace(config.Account)
	if account == "" {
		return nil, &InputError{Field: "Account", Message: "is required"}
	}
	if config.Password == "" {
		return nil, &InputError{Field: "Password", Message: "is required"}
	}
	solvers := make([]CaptchaSolver, 0, len(config.CaptchaSolvers))
	for _, solver := range config.CaptchaSolvers {
		if solver != nil {
			solvers = append(solvers, solver)
		}
	}
	if len(solvers) == 0 {
		return nil, &InputError{Field: "CaptchaSolvers", Message: "at least one solver is required"}
	}
	loginAttempts := config.LoginAttempts
	if loginAttempts <= 0 {
		loginAttempts = defaultLoginAttempts
	}
	if loginAttempts > maximumLoginAttempts {
		return nil, &InputError{Field: "LoginAttempts", Message: "must not exceed 10"}
	}

	jar, saveCookies, err := clientCookieJar(config)
	if err != nil {
		return nil, err
	}
	var httpClient http.Client
	if config.HTTPClient != nil {
		httpClient = *config.HTTPClient
	} else {
		httpClient = http.Client{Timeout: 60 * time.Second, Transport: defaultHTTPTransport()}
	}
	httpClient.Jar = jar

	concurrency := config.PageConcurrency
	if concurrency <= 0 {
		concurrency = defaultPageConcurrency
	}
	return &Client{
		account:         account,
		password:        config.Password,
		captchaSolvers:  solvers,
		httpClient:      &httpClient,
		saveCookies:     saveCookies,
		pageConcurrency: concurrency,
		loginAttempts:   loginAttempts,
		endpoints:       productionEndpoints,
	}, nil
}

func clientCookieJar(config Config) (http.CookieJar, func() error, error) {
	if config.CookieStore != nil {
		if nilCookieStore(config.CookieStore) {
			return nil, nil, &InputError{Field: "CookieStore", Message: "must not be nil"}
		}
		if strings.TrimSpace(config.CookieFile) != "" {
			return nil, nil, &InputError{Field: "CookieStore", Message: "must not be used together with CookieFile"}
		}
		return cookieJarFromStore(config.CookieStore)
	}
	filename := strings.TrimSpace(config.CookieFile)
	if filename != "" {
		if parent := filepath.Dir(filename); parent != "." {
			if err := os.MkdirAll(parent, 0o700); err != nil {
				return nil, nil, fmt.Errorf("create cookie directory: %w", err)
			}
		}
		jar, err := persistentjar.New(&persistentjar.Options{Filename: filename})
		if err != nil {
			return nil, nil, fmt.Errorf("open cookie jar: %w", err)
		}
		save := func() error {
			if err := jar.Save(); err != nil {
				return fmt.Errorf("save cookie jar: %w", err)
			}
			if err := os.Chmod(filename, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("secure cookie jar: %w", err)
			}
			return nil
		}
		return jar, save, nil
	}
	if config.HTTPClient != nil && config.HTTPClient.Jar != nil {
		return config.HTTPClient.Jar, func() error { return nil }, nil
	}
	jar, err := standardjar.New(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create cookie jar: %w", err)
	}
	return jar, func() error { return nil }, nil
}

func defaultHTTPTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 160 store jobs can each run Article View pages and Trend View at once
	// against the same RTA hosts. Keep enough idle sockets so workers do not
	// rebuild TLS sessions on every query.
	transport.MaxIdleConns = defaultMaxIdleConns
	transport.MaxIdleConnsPerHost = defaultMaxIdleConnsPerHost
	transport.MaxConnsPerHost = defaultMaxConnsPerHost
	return transport
}

type requestBuilder func(context.Context) (*http.Request, error)

func (c *Client) doAuthenticated(ctx context.Context, operation string, build requestBuilder) ([]byte, error) {
	observedVersion := c.sessionVersion.Load()
	body, status, err := c.do(ctx, operation, build)
	if err != nil {
		return nil, err
	}
	if !isUnauthenticated(status, body) {
		return checkedHTTPBody(operation, status, body)
	}
	if err := c.ensureLogin(ctx, observedVersion); err != nil {
		return nil, err
	}
	body, status, err = c.do(ctx, operation, build)
	if err != nil {
		return nil, err
	}
	if isUnauthenticated(status, body) {
		code, message := authenticationDetails(body)
		return nil, &AuthError{Code: code, Message: "session remained expired after automatic login: " + message}
	}
	return checkedHTTPBody(operation, status, body)
}

func (c *Client) do(ctx context.Context, operation string, build requestBuilder) ([]byte, int, error) {
	request, err := build(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: create request: %w", operation, err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, &UpstreamError{Operation: operation, Err: err}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, response.StatusCode, &UpstreamError{Operation: operation, StatusCode: response.StatusCode, Err: err}
	}
	if len(body) > maxResponseBytes {
		return nil, response.StatusCode, &ProtocolError{Operation: operation, Message: "response exceeded 64 MiB"}
	}
	return body, response.StatusCode, nil
}

func checkedHTTPBody(operation string, status int, body []byte) ([]byte, error) {
	if status < 200 || status >= 300 {
		return nil, &UpstreamError{Operation: operation, StatusCode: status, Body: compactPreview(string(body))}
	}
	return body, nil
}

func isUnauthenticated(status int, body []byte) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	code, _ := authenticationDetails(body)
	if code == "9800" || code == "401" || code == "403" {
		return true
	}
	text := string(body)
	return strings.Contains(text, "用戶未登錄") ||
		strings.Contains(text, "用户未登录") ||
		strings.Contains(text, "mansso.rta-os.com/login")
}

func authenticationDetails(body []byte) (string, string) {
	envelope, err := decodeEnvelope(body, "authentication")
	if err != nil {
		return "", compactPreview(string(body))
	}
	return string(envelope.Code), envelopeMessage(envelope)
}

func setCommonHeaders(request *http.Request) {
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept-Language", "zh-TW,zh;q=0.8,en-US;q=0.5,en;q=0.3")
}
