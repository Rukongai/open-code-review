// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/alibaba/open-code-review/internal/triage"
)

type fakeTriager struct {
	res  triage.Result
	err  error
	seen string
}

func (f *fakeTriager) Classify(_ context.Context, diff string) (triage.Result, error) {
	f.seen = diff
	return f.res, f.err
}

func TestExecuteSubtask_TriageControlsPlanPhase(t *testing.T) {
	cases := []struct {
		name      string
		triager   *fakeTriager
		wantCalls int
	}{
		{"light skips plan", &fakeTriager{res: triage.Result{Tier: triage.Light}}, 1},
		{"deep keeps plan", &fakeTriager{res: triage.Result{Tier: triage.Deep}}, 2},
		{"failure keeps plan", &fakeTriager{err: errors.New("gateway down")}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			planText := "my plan"
			client := &fakeAgentClient{responses: []*llm.ChatResponse{
				{Choices: []llm.Choice{{Message: llm.ResponseMessage{Content: &planText}}}},
				agentTaskDoneResponse(),
			}}
			if tc.wantCalls == 1 {
				client.responses = client.responses[1:]
			}
			a := New(Args{
				LLMClient: client,
				Model:     "test",
				Session:   session.New(t.TempDir(), "main", "test", session.SessionOptions{ReviewMode: "diff"}),
				Tools:     tool.NewRegistry(),
				Triage:    tc.triager,
				Template: template.Template{
					MaxTokens:           100000,
					MaxToolRequestTimes: 10,
					PlanTask:            &template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Plan for: {{diffs}}"}}},
					MainTask:            template.LlmConversation{Messages: []template.ChatMessage{{Role: "user", Content: "Review {{plan_guidance}}: {{diffs}}"}}},
				},
				MainToolDefs: []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "task_done", Description: "done"}}},
			})
			a.currentDate = "2025-06-26 10:00"
			d := model.Diff{NewPath: "main.go", OldPath: "main.go", Diff: "+new code", Insertions: 5}

			completed, stop, err := a.executeGroupSubtask(context.Background(), FileGroup{Label: d.NewPath, Diffs: []model.Diff{d}})
			if err != nil || stop != nil || !completed {
				t.Fatalf("completed=%v stop=%+v err=%v", completed, stop, err)
			}
			if client.calls != tc.wantCalls {
				t.Errorf("LLM calls = %d, want %d", client.calls, tc.wantCalls)
			}
			if tc.triager.seen != buildConcatenatedDiffs([]model.Diff{d}) {
				t.Errorf("triage input = %q", tc.triager.seen)
			}
		})
	}
}
