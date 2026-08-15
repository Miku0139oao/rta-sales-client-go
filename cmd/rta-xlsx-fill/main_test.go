package main

import (
	"strings"
	"testing"
)

func TestCommandDateRange(t *testing.T) {
	tests := []struct {
		name, date, from, to string
		wantFrom, wantTo     string
		wantError            string
	}{
		{name: "single date", date: "2026-08-15", wantFrom: "2026-08-15", wantTo: "2026-08-15"},
		{name: "inclusive cross month", from: "2026-07-31", to: "2026-08-02", wantFrom: "2026-07-31", wantTo: "2026-08-02"},
		{name: "date conflicts", date: "2026-08-15", from: "2026-08-15", to: "2026-08-15", wantError: "mutually exclusive"},
		{name: "missing from", to: "2026-08-15", wantError: "either -date or both"},
		{name: "missing to", from: "2026-08-15", wantError: "either -date or both"},
		{name: "invalid date", date: "15/08/2026", wantError: "-date must use"},
		{name: "invalid from", from: "no", to: "2026-08-15", wantError: "-from must use"},
		{name: "invalid to", from: "2026-08-15", to: "no", wantError: "-to must use"},
		{name: "reverse", from: "2026-08-16", to: "2026-08-15", wantError: "must not precede"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			from, to, err := commandDateRange(test.date, test.from, test.to)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error=%v, want substring %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := from.Format("2006-01-02"); got != test.wantFrom {
				t.Fatalf("from=%q, want %q", got, test.wantFrom)
			}
			if got := to.Format("2006-01-02"); got != test.wantTo {
				t.Fatalf("to=%q, want %q", got, test.wantTo)
			}
		})
	}
}
