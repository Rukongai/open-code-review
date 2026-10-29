// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package triage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decisionsServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	var got http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r.Clone(context.Background())
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["input"] != "the diff" || req["model"] != "m" {
			t.Errorf("request = %v", req)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func tierBody(skip, light, deep float64) string {
	b, _ := json.Marshal(map[string]any{
		"answers": []any{
			map[string]any{"name": "tier", "probabilities": []any{
				map[string]any{"value": "skip", "probability": skip},
				map[string]any{"value": "light", "probability": light},
				map[string]any{"value": "deep", "probability": deep},
			}},
			map[string]any{"name": "has_issue", "probability": 0.3},
		},
		"usage": map[string]any{"input_tokens": 42},
	})
	return string(b)
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name             string
		skip, light, dep float64
		want             Tier
	}{
		{"deep", 0, 0.02, 0.98, Deep},
		{"light", 0.04, 0.95, 0.01, Light},
		{"skip counts as light", 0.6, 0.3, 0.1, Light},
		{"threshold is deep", 0, 0.5, 0.5, Deep},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := decisionsServer(t, http.StatusOK, tierBody(tc.skip, tc.light, tc.dep))
			c := &Client{URL: srv.URL, Model: "m", Token: "tok", Headers: map[string]string{"x-litellm-tags": "t"}, HTTP: srv.Client()}
			res, err := c.Classify(context.Background(), "the diff")
			if err != nil {
				t.Fatal(err)
			}
			if res.Tier != tc.want || res.PDeep != tc.dep || res.HasIssue != 0.3 || res.InputTokens != 42 {
				t.Errorf("result = %+v", res)
			}
			if got.Header.Get("Authorization") != "Bearer tok" || got.Header.Get("x-litellm-tags") != "t" {
				t.Errorf("headers = %v", got.Header)
			}
		})
	}
}

func TestClassify_Errors(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{"http error", `{"error":"nope"}`, "returned 500", http.StatusInternalServerError},
		{"bad json", `{`, "decode", http.StatusOK},
		{"no tier", `{"answers":[{"name":"has_issue","probability":0.1}]}`, "no deep tier", http.StatusOK},
		{"tier without deep", `{"answers":[{"name":"tier","probabilities":[{"value":"light","probability":0.9}]}]}`, "no deep tier", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := decisionsServer(t, tc.status, tc.body)
			c := &Client{URL: srv.URL, Model: "m", HTTP: srv.Client()}
			if _, err := c.Classify(context.Background(), "the diff"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestClassify_Unreachable(t *testing.T) {
	c := &Client{URL: "http://127.0.0.1:1", Model: "m", HTTP: http.DefaultClient}
	if _, err := c.Classify(context.Background(), "x"); err == nil {
		t.Fatal("want error")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("OCR_TRIAGE_URL", "")
	if FromEnv() != nil {
		t.Fatal("triage should be off without OCR_TRIAGE_URL")
	}
	t.Setenv("OCR_TRIAGE_URL", "http://x/decisions")
	t.Setenv("OCR_TRIAGE_MODEL", "")
	t.Setenv("OCR_TRIAGE_TOKEN", "tok")
	t.Setenv("OCR_TRIAGE_TAGS", "team")
	c := FromEnv()
	if c == nil || c.Model != "gpt-6-luna" || c.Token != "tok" || c.Headers["x-litellm-tags"] != "team" {
		t.Fatalf("client = %+v", c)
	}
}
