// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package agent

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestWhyExcluded_ReportsRule(t *testing.T) {
	tests := []struct {
		name       string
		filter     *rules.FileFilter
		diff       model.Diff
		wantReason ExcludeReason
		wantRule   string
	}{
		{"user exclude keeps the pattern as written", &rules.FileFilter{Exclude: []string{"Docs/**"}}, model.Diff{NewPath: "docs/a.go"}, ExcludeUserRule, "Docs/**"},
		{"default path", nil, model.Diff{NewPath: "pkg/foo_test.go"}, ExcludeDefaultPath, "**/*_test.go"},
		{"extension", nil, model.Diff{NewPath: "notes.txt"}, ExcludeExtension, ".txt"},
		{"dotenv", nil, model.Diff{NewPath: "app/.env.prod"}, ExcludeSecret, ".env*"},
		{"secret on the old path of a rename", nil, model.Diff{OldPath: ".env", NewPath: "config.go"}, ExcludeSecret, ".env*"},
		{"binary has no rule", nil, model.Diff{NewPath: "logo.png", IsBinary: true}, ExcludeBinary, ""},
		{"selected", nil, model.Diff{NewPath: "main.go"}, ExcludeNone, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, rule := New(Args{FileFilter: tt.filter}).whyExcluded(tt.diff)
			if reason != tt.wantReason || rule != tt.wantRule {
				t.Errorf("whyExcluded = (%q, %q), want (%q, %q)", reason, rule, tt.wantReason, tt.wantRule)
			}
		})
	}
}

func TestExcludedFiles(t *testing.T) {
	a := New(Args{Template: template.Template{MaxTokens: 100}})
	big := make([]byte, 4000)
	for i := range big {
		big[i] = 'x'
	}
	decisions := a.selectFiles([]model.Diff{
		{NewPath: "main.go", Diff: "+x"},
		{NewPath: "notes.txt", Diff: "+x"},
		{NewPath: "gone.go", OldPath: "gone.go", IsDeleted: true},
		{NewPath: "huge.go", Diff: "+" + string(big)},
	})
	got := excludedFiles(decisions)
	if len(got) != 3 {
		t.Fatalf("excludedFiles = %+v, want 3 entries", got)
	}
	if got[0] != (ExcludedFile{Path: "notes.txt", Reason: ExcludeExtension, Rule: ".txt"}) {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].Path != "gone.go" || got[1].Reason != ExcludeDeleted {
		t.Errorf("got[1] = %+v", got[1])
	}
	if got[2].Path != "huge.go" || got[2].Reason != ExcludeTooLarge || got[2].Rule == "" {
		t.Errorf("got[2] = %+v", got[2])
	}
	if excludedFiles(a.selectFiles([]model.Diff{{NewPath: "main.go", Diff: "+x"}})) != nil {
		t.Error("a fully selected run should report no exclusions")
	}
}
