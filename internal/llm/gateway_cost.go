// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"math"
	"net/http"
	"strconv"
)

// gatewayCostHeader is the per-response price LiteLLM proxies attach. It is
// the gateway's own figure for the key in use, so it reflects negotiated rates
// and cache discounts that a token-count estimate cannot.
const gatewayCostHeader = "x-litellm-response-cost"

// GatewayCost is the spend reported by an LLM gateway over one run. It is nil
// when no response carried a cost header, so runs against providers without a
// gateway emit nothing.
type GatewayCost struct {
	TotalUSD float64 `json:"total_usd"`
	// Responses counts the responses that carried a cost; a gap against the
	// request count means part of the run is missing from TotalUSD.
	Responses int                `json:"responses"`
	ByTaskUSD map[string]float64 `json:"by_task_usd,omitempty"`
}

type gatewayCostTotals struct {
	total     float64
	responses int
	byTask    map[string]float64
}

func parseGatewayCost(h http.Header) (float64, bool) {
	v := h.Get(gatewayCostHeader)
	if v == "" {
		return 0, false
	}
	cost, err := strconv.ParseFloat(v, 64)
	if err != nil || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return 0, false
	}
	return cost, true
}

// RecordGatewayCost adds one response's reported cost. Unlike RecordAttempt it
// accepts requests without identity, so scan and other unattributed calls still
// count toward the total; they are grouped under "other".
func (c *RetryCollector) RecordGatewayCost(taskType string, cost float64) {
	if c == nil {
		return
	}
	if taskType == "" {
		taskType = "other"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cost.byTask == nil {
		c.cost.byTask = make(map[string]float64)
	}
	c.cost.total += cost
	c.cost.responses++
	c.cost.byTask[taskType] += cost
}

// GatewayCost returns the run's gateway-reported spend, or nil if none was seen.
func (c *RetryCollector) GatewayCost() *GatewayCost {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cost.responses == 0 {
		return nil
	}
	byTask := make(map[string]float64, len(c.cost.byTask))
	for k, v := range c.cost.byTask {
		byTask[k] = v
	}
	return &GatewayCost{TotalUSD: c.cost.total, Responses: c.cost.responses, ByTaskUSD: byTask}
}

// Total is TotalUSD, or zero for a run that reported no cost.
func (g *GatewayCost) Total() float64 {
	if g == nil {
		return 0
	}
	return g.TotalUSD
}
