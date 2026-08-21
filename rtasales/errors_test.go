package rtasales

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestProtocolErrorRetryable(t *testing.T) {
	malformed := json.Unmarshal([]byte("<html>too many requests</html>"), new(any))
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "invalid JSON", err: &ProtocolError{Operation: "sales", Message: "response is not valid JSON", Err: malformed}, retryable: true},
		{name: "empty data", err: &ProtocolError{Operation: "sales", Message: "response data is empty"}, retryable: true},
		{name: "envelope code", err: &ProtocolError{Operation: "sales", Message: "RTA code 429: too many requests"}, retryable: true},
		{name: "invalid sales data", err: &ProtocolError{Operation: "sales", Message: "invalid sales data"}, retryable: false},
		{name: "http 429", err: &UpstreamError{Operation: "sales", StatusCode: 429}, retryable: true},
		{name: "http timeout", err: &UpstreamError{Operation: "sales", Err: context.DeadlineExceeded}, retryable: true},
		{name: "http 403", err: &UpstreamError{Operation: "sales", StatusCode: 403}, retryable: false},
		{name: "permission", err: &StoreNotFoundError{BusinessStoreID: "107"}, retryable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsRetryable(test.err); got != test.retryable {
				t.Fatalf("IsRetryable()=%t, want %t for %v", got, test.retryable, test.err)
			}
		})
	}
	if IsRetryable(nil) {
		t.Fatal("nil error must not be retryable")
	}
	if IsRetryable(errors.New("plain")) {
		t.Fatal("plain errors must not be retryable")
	}
}
