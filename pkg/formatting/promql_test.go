package formatting

import (
	"strings"
	"testing"
)

func TestShouldBeMultiline(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "short simple expression",
			expr:     `up{job="api"}`,
			expected: false,
		},
		{
			name:     "long expression over 80 chars",
			expr:     `sum(rate(http_requests_total{job="api",status=~"5.."}[5m])) by (instance) / sum(rate(http_requests_total{job="api"}[5m])) by (instance)`,
			expected: true,
		},
		{
			name:     "expression with multiple operators",
			expr:     `rate(metric[5m]) and on(instance) other_metric or third_metric`,
			expected: true,
		},
		{
			name:     "simple aggregation",
			expr:     `sum(metric) by (label)`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldBeMultiline(tt.expr)
			if result != tt.expected {
				t.Errorf("shouldBeMultiline(%q) = %v, want %v", tt.expr, result, tt.expected)
			}
		})
	}
}

func TestCheckAndFormatPromQL(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectIssues  bool
		expectChanged bool
	}{
		{
			name: "well formatted expression",
			input: `expr: |
  sum(rate(metric[5m]))`,
			expectIssues:  false,
			expectChanged: false,
		},
		{
			name:          "long single-line expression",
			input:         `expr: sum(rate(http_requests_total{job="api",status=~"5.."}[5m])) by (instance) / sum(rate(http_requests_total{job="api"}[5m])) by (instance)`,
			expectIssues:  true,
			expectChanged: false, // Note: formatting is detected but complex multiline split not yet implemented
		},
		{
			name:          "short expression",
			input:         `expr: up{job="test"}`,
			expectIssues:  false,
			expectChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, formatted := CheckAndFormatPromQL(tt.input)

			if tt.expectIssues && len(issues) == 0 {
				t.Errorf("Expected issues but got none")
			}

			if !tt.expectIssues && len(issues) > 0 {
				t.Errorf("Expected no issues but got: %v", issues)
			}

			if tt.expectChanged && formatted == tt.input {
				t.Errorf("Expected formatting changes but content unchanged")
			}

			if !tt.expectChanged && formatted != tt.input {
				t.Errorf("Expected no changes but content was modified")
			}
		})
	}
}

func TestGetIndentation(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "no indentation",
			line:     "expr: test",
			expected: "",
		},
		{
			name:     "two spaces",
			line:     "  expr: test",
			expected: "  ",
		},
		{
			name:     "four spaces",
			line:     "    expr: test",
			expected: "    ",
		},
		{
			name:     "tab",
			line:     "\texpr: test",
			expected: "\t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIndentation(tt.line)
			if result != tt.expected {
				t.Errorf("getIndentation(%q) = %q, want %q", tt.line, result, tt.expected)
			}
		})
	}
}

func TestIsOperator(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"and operator", "and", true},
		{"or operator", "or", true},
		{"unless operator", "unless", true},
		{"by clause", "by(", true},
		{"without clause", "without(", true},
		{"metric name", "http_requests_total", false},
		{"number", "123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOperator(tt.input)
			if result != tt.expected {
				t.Errorf("isOperator(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatYAMLBlock(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		expr        string
		indentation string
		wantMulti   bool
	}{
		{
			name:        "single line expression",
			prefix:      "expr:",
			expr:        "up",
			indentation: "  ",
			wantMulti:   false,
		},
		{
			name:        "multiline expression",
			prefix:      "expr:",
			expr:        "line1\nline2",
			indentation: "  ",
			wantMulti:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatYAMLBlock(tt.prefix, tt.expr, tt.indentation)

			if tt.wantMulti {
				if !strings.Contains(result, "|") {
					t.Errorf("Expected multiline indicator '|' in result")
				}
				if !strings.Contains(result, "\n") {
					t.Errorf("Expected newlines in multiline result")
				}
			} else {
				if strings.Contains(result, "|") {
					t.Errorf("Did not expect multiline indicator '|' for single line")
				}
			}
		})
	}
}
