package rtasales

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxTransientAttempts = 3
	maxRetryAfter        = 30 * time.Second
)

var transientRetryDelays = [...]time.Duration{time.Second, 3 * time.Second}

func (c *Client) wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	if c.retryWait != nil {
		return c.retryWait(ctx, delay)
	}
	return waitForRetry(ctx, delay)
}

func (c *Client) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func shouldRetry(ctx context.Context, attempt int, status int, err error, retryAfter time.Duration) (bool, time.Duration) {
	if attempt >= maxTransientAttempts-1 {
		return false, 0
	}
	delay := transientRetryDelays[attempt]
	if retryAfter > 0 {
		delay = retryAfter
		if delay > maxRetryAfter {
			delay = maxRetryAfter
		}
	}
	if err != nil {
		if retryableTransport(ctx, err) {
			if retryAfter <= 0 {
				delay = transientRetryDelays[attempt]
			}
			return true, delay
		}
		return false, 0
	}
	if retryableStatus(status) {
		if retryAfter <= 0 {
			delay = transientRetryDelays[attempt]
		}
		return true, delay
	}
	return false, 0
}

func retryableTransport(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	var upstream *UpstreamError
	if errors.As(err, &upstream) && upstream.Err != nil {
		err = upstream.Err
	}
	if ctx.Err() != nil && (errors.Is(err, ctx.Err()) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, needle := range []string{
		"timeout",
		"temporar",
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"server closed idle connection",
		"connection closed",
		"forcibly closed",
		"wsarecv",
		"i/o timeout",
		"tls handshake timeout",
		"use of closed network connection",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		delay := time.Duration(seconds) * time.Second
		if delay > maxRetryAfter {
			return maxRetryAfter
		}
		return delay
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	delay := when.Sub(now)
	if delay <= 0 {
		return 0
	}
	if delay > maxRetryAfter {
		return maxRetryAfter
	}
	return delay
}

func rateLimitedBody(body []byte) bool {
	envelope, err := decodeEnvelope(body, "rate-limit")
	if err != nil {
		text := string(body)
		lower := strings.ToLower(text)
		return strings.Contains(lower, "too many requests") ||
			strings.Contains(text, "请求过于频繁") ||
			strings.Contains(text, "請求過於頻繁")
	}
	code := string(envelope.Code)
	if successfulCode(code) {
		return false
	}
	message := envelopeMessage(envelope)
	if permissionDenied(code, message) {
		return false
	}
	if code == "429" || code == "408" {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(message, "频繁") ||
		strings.Contains(message, "頻繁") ||
		strings.Contains(lower, "too many") ||
		strings.Contains(lower, "rate limit")
}

func permissionDenied(code, message string) bool {
	switch strings.TrimSpace(code) {
	case "401", "403":
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(message, "没有权限") ||
		strings.Contains(message, "沒有權限") ||
		strings.Contains(message, "无权") ||
		strings.Contains(message, "無權") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access denied")
}
