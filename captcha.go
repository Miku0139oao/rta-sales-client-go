package rtasales

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CaptchaSolver converts an RTA captcha image into its answer. Solvers are
// attempted in Config.CaptchaSolvers order.
type CaptchaSolver interface {
	Solve(context.Context, []byte) (string, error)
}

// TesseractConfig configures local OCR. The zero value uses the tesseract
// command, page segmentation mode 7, and accepts 4-6 alphanumeric characters.
type TesseractConfig struct {
	Command   string
	Language  string
	PSM       int
	Whitelist string
	MinLength int
	MaxLength int
}

// TesseractSolver runs the local Tesseract executable. It has no CGO
// dependency; deployments only need the tesseract binary available at runtime.
type TesseractSolver struct {
	config TesseractConfig
}

func NewTesseractSolver(config TesseractConfig) *TesseractSolver {
	if strings.TrimSpace(config.Command) == "" {
		config.Command = "tesseract"
	}
	if config.PSM <= 0 {
		config.PSM = 7
	}
	if config.Whitelist == "" {
		config.Whitelist = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	}
	if config.MinLength <= 0 {
		config.MinLength = 4
	}
	if config.MaxLength <= 0 {
		config.MaxLength = 6
	}
	return &TesseractSolver{config: config}
}

func (s *TesseractSolver) Solve(ctx context.Context, image []byte) (string, error) {
	if len(image) == 0 {
		return "", errors.New("captcha image is empty")
	}
	arguments := []string{
		"stdin", "stdout",
		"--psm", strconv.Itoa(s.config.PSM),
		"-c", "tessedit_char_whitelist=" + s.config.Whitelist,
	}
	if strings.TrimSpace(s.config.Language) != "" {
		arguments = append(arguments, "-l", strings.TrimSpace(s.config.Language))
	}
	command := exec.CommandContext(ctx, s.config.Command, arguments...)
	command.Stdin = bytes.NewReader(image)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("run Tesseract OCR: %s", message)
	}
	answer := normalizeCaptcha(string(output), s.config.Whitelist)
	if len(answer) < s.config.MinLength || len(answer) > s.config.MaxLength {
		return "", fmt.Errorf("tesseract OCR returned %d usable characters, expected %d-%d", len(answer), s.config.MinLength, s.config.MaxLength)
	}
	return answer, nil
}

func normalizeCaptcha(value, whitelist string) string {
	allowed := make(map[rune]struct{}, len(whitelist))
	for _, character := range whitelist {
		allowed[character] = struct{}{}
	}
	var output strings.Builder
	for _, character := range strings.TrimSpace(value) {
		if _, ok := allowed[character]; ok {
			output.WriteRune(character)
			continue
		}
		upper := []rune(strings.ToUpper(string(character)))
		if len(upper) == 1 {
			if _, ok := allowed[upper[0]]; ok {
				output.WriteRune(upper[0])
			}
		}
	}
	return output.String()
}

// TwoCaptchaConfig configures the paid fallback solver.
type TwoCaptchaConfig struct {
	APIKey       string
	HTTPClient   *http.Client
	PollInterval time.Duration
	Timeout      time.Duration
	BaseURL      string
}

// TwoCaptchaSolver solves normal image captchas using 2Captcha.
type TwoCaptchaSolver struct {
	apiKey       string
	httpClient   *http.Client
	pollInterval time.Duration
	timeout      time.Duration
	baseURL      string
}

// NewTwoCaptchaSolver returns a solver with production defaults.
func NewTwoCaptchaSolver(apiKey string) *TwoCaptchaSolver {
	return NewTwoCaptchaSolverWithConfig(TwoCaptchaConfig{APIKey: apiKey})
}

// NewTwoCaptchaSolverWithConfig allows callers to tune polling and use a
// custom HTTP client. BaseURL is primarily useful for compatible services and
// deterministic tests.
func NewTwoCaptchaSolverWithConfig(config TwoCaptchaConfig) *TwoCaptchaSolver {
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	pollInterval := config.PollInterval
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://2captcha.com"
	}
	return &TwoCaptchaSolver{
		apiKey:       strings.TrimSpace(config.APIKey),
		httpClient:   client,
		pollInterval: pollInterval,
		timeout:      timeout,
		baseURL:      baseURL,
	}
}

type twoCaptchaResponse struct {
	Status  flexibleString `json:"status"`
	Request string         `json:"request"`
}

func (s *TwoCaptchaSolver) Solve(ctx context.Context, image []byte) (string, error) {
	if s.apiKey == "" {
		return "", errors.New("2Captcha API key is required")
	}
	if len(image) == 0 {
		return "", errors.New("captcha image is empty")
	}
	form := url.Values{
		"key":    {s.apiKey},
		"method": {"base64"},
		"body":   {base64.StdEncoding.EncodeToString(image)},
		"json":   {"1"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/in.php", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("submit captcha to 2Captcha: %w", err)
	}
	submission, err := decodeTwoCaptcha(response, "submit")
	if err != nil {
		return "", err
	}
	if string(submission.Status) != "1" {
		return "", fmt.Errorf("2Captcha submit failed: %s", submission.Request)
	}
	return s.poll(ctx, submission.Request)
}

func (s *TwoCaptchaSolver) poll(ctx context.Context, captchaID string) (string, error) {
	deadline := time.Now().Add(s.timeout)
	for {
		if !time.Now().Before(deadline) {
			return "", errors.New("2Captcha solve timed out")
		}
		timer := time.NewTimer(s.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
		parameters := url.Values{
			"key":    {s.apiKey},
			"action": {"get"},
			"id":     {captchaID},
			"json":   {"1"},
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/res.php?"+parameters.Encode(), nil)
		if err != nil {
			return "", err
		}
		response, err := s.httpClient.Do(request)
		if err != nil {
			return "", fmt.Errorf("poll 2Captcha: %w", err)
		}
		result, err := decodeTwoCaptcha(response, "poll")
		if err != nil {
			return "", err
		}
		if string(result.Status) == "1" {
			answer := strings.TrimSpace(result.Request)
			if answer == "" {
				return "", errors.New("2Captcha returned an empty answer")
			}
			return answer, nil
		}
		if result.Request != "CAPCHA_NOT_READY" {
			return "", fmt.Errorf("2Captcha solve failed: %s", result.Request)
		}
	}
}

func decodeTwoCaptcha(response *http.Response, operation string) (twoCaptchaResponse, error) {
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return twoCaptchaResponse{}, fmt.Errorf("read 2Captcha %s response: %w", operation, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return twoCaptchaResponse{}, fmt.Errorf("2Captcha %s returned HTTP %d: %s", operation, response.StatusCode, compactPreview(string(body)))
	}
	var result twoCaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("decode 2Captcha %s response: %w", operation, err)
	}
	if strings.TrimSpace(result.Request) == "" {
		return result, fmt.Errorf("2Captcha %s returned an empty response", operation)
	}
	return result, nil
}
