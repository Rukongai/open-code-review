// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package triage asks the OpenAI Decisions API how much review effort a file
// group needs, so low-risk groups can skip the plan phase and extra rounds.
package triage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Tier string

const (
	Light Tier = "light"
	Deep  Tier = "deep"
)

// deepThreshold is deliberately low: a missed deep review costs findings,
// while a needless one only costs money.
const deepThreshold = 0.5

type Result struct {
	Tier        Tier
	PSkip       float64
	PLight      float64
	PDeep       float64
	HasIssue    float64
	InputTokens int
}

type Client struct {
	URL     string
	Model   string
	Token   string
	Headers map[string]string
	HTTP    *http.Client
}

// FromEnv returns nil unless OCR_TRIAGE_URL is set, which keeps triage off by default.
func FromEnv() *Client {
	url := os.Getenv("OCR_TRIAGE_URL")
	if url == "" {
		return nil
	}
	c := &Client{
		URL:   url,
		Model: os.Getenv("OCR_TRIAGE_MODEL"),
		Token: os.Getenv("OCR_TRIAGE_TOKEN"),
		HTTP:  &http.Client{Timeout: 60 * time.Second},
	}
	if c.Model == "" {
		c.Model = "gpt-6-luna"
	}
	if tags := os.Getenv("OCR_TRIAGE_TAGS"); tags != "" {
		c.Headers = map[string]string{"x-litellm-tags": tags}
	}
	return c
}

var questions = []map[string]any{
	{
		"type":         "choice",
		"name":         "tier",
		"instructions": "The input is one group of files from a pull request diff. Choose how much automated code review effort this group needs.",
		"choices": []map[string]string{
			{"value": "skip", "description": "No defect worth a reviewer comment is plausible: formatting, comments or docs, pure renames, generated or vendored output, or plain style values with no logic."},
			{"value": "light", "description": "Low risk; one quick pass is enough: presentational markup, simple styling, straightforward glue code, small config."},
			{"value": "deep", "description": "Could break correctness, security, data integrity, concurrency, accessibility or licensing: business logic, data access, auth, migrations, schemas, async or lifecycle code, browser APIs, external integrations."},
		},
	},
	{
		"type":         "predicate",
		"name":         "has_issue",
		"instructions": "An experienced reviewer would leave at least one comment about a real bug, security problem, data-integrity problem, resource leak, accessibility problem or licensing problem in this diff.",
	},
}

type response struct {
	Answers []struct {
		Name          string  `json:"name"`
		Probability   float64 `json:"probability"`
		Probabilities []struct {
			Value       string  `json:"value"`
			Probability float64 `json:"probability"`
		} `json:"probabilities"`
	} `json:"answers"`
	Usage struct {
		InputTokens int `json:"input_tokens"`
	} `json:"usage"`
}

func (c *Client) Classify(ctx context.Context, diff string) (Result, error) {
	body, err := json.Marshal(map[string]any{"model": c.Model, "input": diff, "questions": questions})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("decisions API returned %d: %.300s", resp.StatusCode, raw)
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Result{}, fmt.Errorf("decode decisions response: %w", err)
	}
	res := Result{InputTokens: r.Usage.InputTokens}
	tierSeen, deepSeen := false, false
	for _, a := range r.Answers {
		switch a.Name {
		case "tier":
			tierSeen = true
			for _, p := range a.Probabilities {
				switch p.Value {
				case "skip":
					res.PSkip = p.Probability
				case "light":
					res.PLight = p.Probability
				case "deep":
					res.PDeep, deepSeen = p.Probability, true
				}
			}
		case "has_issue":
			res.HasIssue = a.Probability
		}
	}
	// Without a deep probability PDeep would read as 0 and quietly mean light.
	if !tierSeen || !deepSeen {
		return Result{}, fmt.Errorf("decisions response has no deep tier probability")
	}
	res.Tier = Deep
	if res.PDeep < deepThreshold {
		res.Tier = Light
	}
	return res, nil
}
