package rtasales

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) ensureLogin(ctx context.Context, observedVersion uint64) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.sessionVersion.Load() != observedVersion {
		return nil
	}
	return c.login(ctx)
}

func (c *Client) login(ctx context.Context) error {
	solverStart := 0
	var lastError error
	for attempt := 0; attempt < c.loginAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		image, flag, err := c.fetchCaptcha(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastError = err
			continue
		}
		answer, usedSolver, err := c.solveCaptcha(ctx, image, solverStart)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastError = err
			continue
		}
		envelope, err := c.submitLogin(ctx, answer, flag)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastError = err
			continue
		}
		code := string(envelope.Code)
		if successfulCode(code) {
			if err := c.saveCookies(); err != nil {
				return err
			}
			c.sessionVersion.Add(1)
			return nil
		}
		message := envelopeMessage(envelope)
		if captchaRetryable(code, message) {
			lastError = &AuthError{Code: code, Message: message}
			if usedSolver+1 < len(c.captchaSolvers) {
				solverStart = usedSolver + 1
			} else {
				solverStart = usedSolver
			}
			continue
		}
		return &AuthError{Code: code, Message: message}
	}
	if lastError == nil {
		lastError = errors.New("login attempts exhausted")
	}
	var captchaError *CaptchaError
	if errors.As(lastError, &captchaError) {
		return captchaError
	}
	return fmt.Errorf("RTA login failed after %d attempts: %w", c.loginAttempts, lastError)
}

func (c *Client) solveCaptcha(ctx context.Context, image []byte, start int) (string, int, error) {
	if start < 0 || start >= len(c.captchaSolvers) {
		start = 0
	}
	errorsBySolver := make([]error, 0, len(c.captchaSolvers)-start)
	for index := start; index < len(c.captchaSolvers); index++ {
		if err := ctx.Err(); err != nil {
			return "", index, err
		}
		answer, err := c.captchaSolvers[index].Solve(ctx, image)
		answer = strings.TrimSpace(answer)
		if err != nil && ctx.Err() != nil {
			return "", index, ctx.Err()
		}
		if err == nil && answer != "" && len([]rune(answer)) <= 16 {
			return answer, index, nil
		}
		if err == nil {
			err = errors.New("solver returned an empty or invalid answer")
		}
		errorsBySolver = append(errorsBySolver, fmt.Errorf("solver %d: %w", index+1, err))
	}
	return "", start, &CaptchaError{Err: errors.Join(errorsBySolver...)}
}

func (c *Client) fetchCaptcha(ctx context.Context) ([]byte, string, error) {
	flag, err := newVerifyCodeFlag()
	if err != nil {
		return nil, "", err
	}
	endpoint := c.endpoints.sso + "/getVerifyCodeImg?verifyCodeFlag=" + url.QueryEscape(flag)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	setCommonHeaders(request)
	request.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	request.Header.Set("Referer", "https://sso.rta-os.com/")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, "", &UpstreamError{Operation: "fetch captcha", Err: err}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, "", &UpstreamError{Operation: "fetch captcha", StatusCode: response.StatusCode, Err: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", &UpstreamError{Operation: "fetch captcha", StatusCode: response.StatusCode, Body: compactPreview(string(body))}
	}
	if len(body) == 0 {
		return nil, "", &ProtocolError{Operation: "fetch captcha", Message: "captcha image is empty"}
	}
	return body, flag, nil
}

func (c *Client) submitLogin(ctx context.Context, answer, flag string) (rtaEnvelope, error) {
	parameters := url.Values{
		"redirectURL":    {"https://partner.rta-os.com/#index/partner/index"},
		"account":        {c.account},
		"password":       {passwordDigest(c.password)},
		"verifyCode":     {answer},
		"verifyCodeFlag": {flag},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoints.sso+"/doLogin?"+parameters.Encode(), nil)
	if err != nil {
		return rtaEnvelope{}, err
	}
	setCommonHeaders(request)
	request.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	request.Header.Set("Origin", "https://sso.rta-os.com")
	request.Header.Set("Referer", "https://sso.rta-os.com/")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return rtaEnvelope{}, &UpstreamError{Operation: "login", Err: err}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return rtaEnvelope{}, &UpstreamError{Operation: "login", StatusCode: response.StatusCode, Err: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return rtaEnvelope{}, &UpstreamError{Operation: "login", StatusCode: response.StatusCode, Body: compactPreview(string(body))}
	}
	return decodeEnvelope(body, "login")
}

func passwordDigest(password string) string {
	md5Hash := md5.Sum([]byte(password))
	md5Hex := hex.EncodeToString(md5Hash[:])
	shaHash := sha256.Sum256([]byte(md5Hex))
	return hex.EncodeToString(shaHash[:])
}

func newVerifyCodeFlag() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate captcha flag: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(value)), nil
}

func captchaRetryable(code, message string) bool {
	return code == "2020350001" ||
		code == "2020350002" ||
		strings.Contains(message, "驗証碼過期") ||
		strings.Contains(message, "验证码过期") ||
		strings.Contains(message, "驗証碼錯誤") ||
		strings.Contains(message, "验证码错误")
}
