// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"strings"
)

const (
	fileReadMaxLines = 500
	// Long enough that hand-written code is never split, short enough that a
	// minified line cannot fill the context; longer lines are paged by start_col.
	fileReadMaxLineBytes   = 32 * 1024
	fileReadMaxOutputBytes = 256 * 1024
)

// FileReadProvider reads file content at a given path and optional line range.
type FileReadProvider struct {
	FileReader *FileReader
}

func NewFileRead(fr *FileReader) *FileReadProvider { return &FileReadProvider{FileReader: fr} }

func (p *FileReadProvider) Tool() Tool { return FileRead }

func (p *FileReadProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return "Error: file_path is required", nil
	}

	startLine, hasStart := args["start_line"].(float64)
	endLine, hasEnd := args["end_line"].(float64)
	if !hasStart || startLine <= 0 {
		startLine = 1
	}
	if !hasEnd || endLine <= 0 {
		endLine = 0
	}
	startCol := 1
	if c, ok := args["start_col"].(float64); ok && c > 1 {
		// Converting an out-of-range float to int is implementation-defined and
		// can go negative; any column past this bound is past every line anyway.
		startCol = int(min(c, 1<<31))
	}

	maxLines := fileReadMaxLines
	if endLine > 0 {
		requested := int(endLine) - int(startLine) + 1
		if requested <= 0 {
			return "", fmt.Errorf("invalid line range: start_line %d is greater than end_line %d", int(startLine), int(endLine))
		}
		if requested < maxLines {
			maxLines = requested
		}
	}

	lines, totalLines, err := p.FileReader.ReadLines(ctx, filePath, int(startLine), maxLines)
	if err != nil {
		return "", fmt.Errorf("file %q not found: %w", filePath, err)
	}

	if totalLines > 0 && int(startLine)-1 >= totalLines {
		return "", fmt.Errorf("file %q has only %d lines, requested range %d-%d", filePath, totalLines, int(startLine), int(endLine))
	}

	effectiveEnd := totalLines
	if endLine > 0 && int(endLine) < effectiveEnd {
		effectiveEnd = int(endLine)
	}
	fullRange := effectiveEnd - (int(startLine) - 1)
	truncated := fullRange > fileReadMaxLines

	var body strings.Builder
	shown := 0
	for i, line := range lines {
		lineNum := int(startLine) + i
		if i > 0 && body.Len() >= fileReadMaxOutputBytes {
			truncated = true
			break
		}
		body.WriteString(fmt.Sprintf("%d|%s\n", lineNum, pageLine(line, lineNum, startCol)))
		shown++
	}
	displayEnd := int(startLine) - 1 + shown

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (Total lines: %d)\n", filePath, totalLines))
	sb.WriteString(fmt.Sprintf("IS_TRUNCATED: %t\n", truncated))
	sb.WriteString(fmt.Sprintf("LINE_RANGE: %d-%d\n", int(startLine), displayEnd))
	sb.WriteString(body.String())
	if shown < len(lines) {
		sb.WriteString(fmt.Sprintf("\nNote: Output limit reached; continue with start_line=%d.\n", displayEnd+1))
	} else if truncated {
		sb.WriteString(fmt.Sprintf("\nNote: Results truncated to %d lines. Please narrow your line range.\n", fileReadMaxLines))
	}
	return sb.String(), nil
}

// pageLine returns line from byte column startCol (1-based), cut to
// fileReadMaxLineBytes with a marker giving the start_col of the next page.
func pageLine(line string, lineNum, startCol int) string {
	from := min(runeBoundary(line, startCol-1), len(line))
	rest := line[from:]
	if len(rest) <= fileReadMaxLineBytes {
		return rest
	}
	cut := runeBoundary(rest, fileReadMaxLineBytes)
	next := from + cut + 1
	return fmt.Sprintf("%s …[line %d continues: %d more bytes; file_read with start_line=%d, end_line=%d, start_col=%d reads the next part]",
		rest[:cut], lineNum, len(rest)-cut, lineNum, lineNum, next)
}
