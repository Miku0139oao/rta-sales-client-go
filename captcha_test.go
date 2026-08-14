package rtasales

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizeCaptcha(t *testing.T) {
	if got := normalizeCaptcha(" aB-12\n", "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"); got != "aB12" {
		t.Fatalf("normalizeCaptcha=%q, want aB12", got)
	}
}

func TestTwoCaptchaSolverSubmitsAndPolls(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/in.php":
			if request.Method != http.MethodPost {
				t.Errorf("submit method=%s", request.Method)
			}
			_ = request.ParseForm()
			if request.Form.Get("key") != "key" || request.Form.Get("body") == "" {
				t.Errorf("invalid submit form: %v", request.Form)
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"status": 1, "request": "captcha-id"})
		case "/res.php":
			polls++
			if polls == 1 {
				_ = json.NewEncoder(response).Encode(map[string]any{"status": 0, "request": "CAPCHA_NOT_READY"})
				return
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"status": "1", "request": "AB12"})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	solver := NewTwoCaptchaSolverWithConfig(TwoCaptchaConfig{
		APIKey:       "key",
		BaseURL:      server.URL,
		PollInterval: time.Millisecond,
		Timeout:      time.Second,
	})
	answer, err := solver.Solve(context.Background(), []byte("image"))
	if err != nil {
		t.Fatal(err)
	}
	if answer != "AB12" || polls != 2 {
		t.Fatalf("answer=%q polls=%d, want AB12/2", answer, polls)
	}
}

func TestTesseractMissingBinaryReturnsFallbackEligibleError(t *testing.T) {
	solver := NewTesseractSolver(TesseractConfig{Command: "definitely-not-an-installed-tesseract-binary"})
	if _, err := solver.Solve(context.Background(), []byte("image")); err == nil {
		t.Fatal("expected missing binary error")
	}
}
