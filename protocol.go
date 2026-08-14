package rtasales

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type flexibleString string

func (s *flexibleString) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = flexibleString(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*s = flexibleString(number.String())
		return nil
	}
	return fmt.Errorf("expected string or number, got %s", string(data))
}

type rtaEnvelope struct {
	Code   flexibleString  `json:"code"`
	Data   json.RawMessage `json:"data"`
	Result json.RawMessage `json:"result"`
	Msg    string          `json:"msg"`
}

func decodeEnvelope(body []byte, operation string) (rtaEnvelope, error) {
	var envelope rtaEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return envelope, &ProtocolError{Operation: operation, Message: "response is not valid JSON", Err: err}
	}
	return envelope, nil
}

func envelopeMessage(envelope rtaEnvelope) string {
	if strings.TrimSpace(envelope.Msg) != "" {
		return strings.TrimSpace(envelope.Msg)
	}
	if len(envelope.Result) == 0 || bytes.Equal(envelope.Result, []byte("null")) {
		return "RTA rejected the request"
	}
	var text string
	if json.Unmarshal(envelope.Result, &text) == nil && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return compactPreview(string(envelope.Result))
}

func successfulCode(code string) bool { return code == "0000" || code == "0" }

func compactPreview(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	if value == "" {
		return "<empty>"
	}
	if len(value) > 500 {
		return value[:500] + "..."
	}
	return value
}

func stringFrom(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func floatFrom(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed
	default:
		return 0
	}
}

func optionalFloatFrom(value any) *float64 {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(stringFrom(value))
	if text == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}
