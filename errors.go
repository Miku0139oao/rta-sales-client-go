package rtasales

import "fmt"

// InputError reports an invalid public API argument.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

// AuthError reports that RTA authentication failed or remained expired after
// the automatic login retry.
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	if e.Code == "" {
		return "RTA authentication failed: " + e.Message
	}
	return fmt.Sprintf("RTA authentication failed (code %s): %s", e.Code, e.Message)
}

// StoreNotFoundError reports that the authenticated account cannot access the
// requested business-facing store ID.
type StoreNotFoundError struct {
	BusinessStoreID string
}

func (e *StoreNotFoundError) Error() string {
	return fmt.Sprintf("RTA account does not have access to business store %q", e.BusinessStoreID)
}

// UpstreamError reports an HTTP or transport failure while calling RTA.
type UpstreamError struct {
	Operation  string
	StatusCode int
	Body       string
	Err        error
}

func (e *UpstreamError) Error() string {
	switch {
	case e.Err != nil:
		return fmt.Sprintf("%s: RTA request failed: %v", e.Operation, e.Err)
	case e.StatusCode != 0:
		return fmt.Sprintf("%s: RTA returned HTTP %d: %s", e.Operation, e.StatusCode, e.Body)
	default:
		return fmt.Sprintf("%s: RTA request failed", e.Operation)
	}
}

func (e *UpstreamError) Unwrap() error { return e.Err }

// Retryable indicates whether a bounded retry is generally safe. Authentication
// errors use AuthError instead and are retried internally once.
func (e *UpstreamError) Retryable() bool {
	return e.Err != nil || e.StatusCode == 408 || e.StatusCode == 429 || e.StatusCode >= 500
}

// ProtocolError reports an incompatible or malformed RTA response.
type ProtocolError struct {
	Operation string
	Message   string
	Err       error
}

func (e *ProtocolError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: invalid RTA response: %s: %v", e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: invalid RTA response: %s", e.Operation, e.Message)
}

func (e *ProtocolError) Unwrap() error { return e.Err }

// CaptchaError reports that every configured captcha solver failed.
type CaptchaError struct {
	Err error
}

func (e *CaptchaError) Error() string {
	if e.Err == nil {
		return "captcha solving failed"
	}
	return "captcha solving failed: " + e.Err.Error()
}

func (e *CaptchaError) Unwrap() error { return e.Err }
