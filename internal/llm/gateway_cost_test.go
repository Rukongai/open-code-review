// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseGatewayCost(t *testing.T) {
	cases := []struct {
		value string
		want  float64
		ok    bool
	}{
		{"", 0, false},
		{"0.0123", 0.0123, true},
		{"0", 0, true},
		{"abc", 0, false},
		{"-1", 0, false},
		{"NaN", 0, false},
		{"+Inf", 0, false},
	}
	for _, tc := range cases {
		h := http.Header{}
		if tc.value != "" {
			h.Set(gatewayCostHeader, tc.value)
		}
		got, ok := parseGatewayCost(h)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseGatewayCost(%q) = %v, %v; want %v, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestGatewayCost_NilWhenNothingReported(t *testing.T) {
	var nilCollector *RetryCollector
	nilCollector.RecordGatewayCost("main_task", 1)
	if nilCollector.GatewayCost() != nil {
		t.Error("nil collector reported a cost")
	}
	if NewRetryCollector().GatewayCost() != nil {
		t.Error("empty collector reported a cost")
	}
	var g *GatewayCost
	if g.Total() != 0 {
		t.Error("nil GatewayCost total is not zero")
	}
}

// The cost header is read on every attempt that returns one, attributed by task,
// and requests without identity (scan, llm test) still count toward the total.
func TestObserverRecordsGatewayCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(gatewayCostHeader, "0.25")
		_, _ = fmt.Fprint(w, anthropicOKBody)
	}))
	defer server.Close()

	c := NewRetryCollector()
	client := anthropicClient(server.URL, c, nil)
	m := testMeta()
	for range 2 {
		if _, err := ping(metaCtx(m), client); err != nil {
			t.Fatalf("CompletionsWithCtx: %v", err)
		}
	}
	if _, err := ping(context.Background(), client); err != nil {
		t.Fatalf("CompletionsWithCtx without meta: %v", err)
	}

	got := c.GatewayCost()
	if got == nil || got.TotalUSD != 0.75 || got.Responses != 3 {
		t.Fatalf("GatewayCost = %+v, want 0.75 over 3 responses", got)
	}
	if got.ByTaskUSD[m.TaskType] != 0.5 || got.ByTaskUSD["other"] != 0.25 {
		t.Errorf("ByTaskUSD = %v", got.ByTaskUSD)
	}
	got.ByTaskUSD["other"] = 99
	if c.GatewayCost().ByTaskUSD["other"] != 0.25 {
		t.Error("GatewayCost returned the collector's own map")
	}
}
