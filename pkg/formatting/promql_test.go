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
			result := shouldBeMultiline(tt.expr, false)
			if result != tt.expected {
				t.Errorf("shouldBeMultiline(%q) = %v, want %v", tt.expr, result, tt.expected)
			}
		})
	}
}

func TestShouldBeMultilineDisabled(t *testing.T) {
	// Test that line length checking can be disabled
	longExpr := `sum(rate(http_requests_total{job="api",status=~"5.."}[5m])) by (instance) / sum(rate(http_requests_total{job="api"}[5m])) by (instance)`

	// With line length enabled, should be true
	if !shouldBeMultiline(longExpr, false) {
		t.Error("Expected true when line length check is enabled")
	}

	// With line length disabled, should still be true (has 2 'by' operators)
	if !shouldBeMultiline(longExpr, true) {
		t.Error("Expected true even with line length disabled (expression has multiple operators)")
	}

	// Simple short expression should be false with line length disabled
	shortExpr := "up{job=\"test\"}"
	if shouldBeMultiline(shortExpr, true) {
		t.Error("Expected false for simple expression when line length check is disabled")
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
			opts := CheckOptions{DisableLineLength: false}
			issues, formatted := CheckAndFormatPromQL(tt.input, opts)

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
			} else if strings.Contains(result, "|") {
				t.Errorf("Did not expect multiline indicator '|' for single line")
			}
		})
	}
}

func TestCheckMetricNamingConventions(t *testing.T) {
	tests := []struct {
		name        string
		metricName  string
		expectIssue bool
	}{
		{
			name:        "valid metric with prefix",
			metricName:  "http_requests_total",
			expectIssue: false,
		},
		{
			name:        "camelCase should fail",
			metricName:  "httpRequestsTotal",
			expectIssue: true,
		},
		{
			name:        "missing prefix",
			metricName:  "requests",
			expectIssue: true,
		},
		{
			name:        "metric with colon (recording rule)",
			metricName:  "job:http_requests:rate5m",
			expectIssue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := checkMetricNamingConventions(tt.metricName)
			if tt.expectIssue && len(issues) == 0 {
				t.Errorf("Expected issues for '%s' but got none", tt.metricName)
			}
			if !tt.expectIssue && len(issues) > 0 {
				t.Errorf("Expected no issues for '%s' but got: %v", tt.metricName, issues)
			}
		})
	}
}

func TestCheckMetricSuffixes(t *testing.T) {
	tests := []struct {
		name        string
		metricName  string
		expectIssue bool
		issueType   string
	}{
		{
			name:        "counter without _total",
			metricName:  "http_requests_count",
			expectIssue: true,
			issueType:   "missing _total",
		},
		{
			name:        "counter with _total",
			metricName:  "http_requests_total",
			expectIssue: false,
		},
		{
			name:        "milliseconds instead of seconds",
			metricName:  "http_duration_milliseconds",
			expectIssue: true,
			issueType:   "non-base unit",
		},
		{
			name:        "seconds (base unit)",
			metricName:  "http_duration_seconds",
			expectIssue: false,
		},
		{
			name:        "percentage instead of ratio",
			metricName:  "cpu_usage_percent",
			expectIssue: true,
			issueType:   "percentage",
		},
		{
			name:        "ratio (correct)",
			metricName:  "cpu_usage_ratio",
			expectIssue: false,
		},
		{
			name:        "megabytes instead of bytes",
			metricName:  "memory_usage_megabytes",
			expectIssue: true,
			issueType:   "non-base unit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := checkMetricSuffixes(tt.metricName)
			if tt.expectIssue && len(issues) == 0 {
				t.Errorf("Expected %s issue for '%s' but got none", tt.issueType, tt.metricName)
			}
			if !tt.expectIssue && len(issues) > 0 {
				t.Errorf("Expected no issues for '%s' but got: %v", tt.metricName, issues)
			}
		})
	}
}

func TestCheckInstrumentationPatterns(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		expectIssue bool
		issueType   string
	}{
		{
			name:        "rate on counter (correct)",
			expr:        "rate(http_requests_total[5m])",
			expectIssue: false,
		},
		{
			name:        "division without protection",
			expr:        "sum(a) / sum(b)",
			expectIssue: true,
			issueType:   "division without zero-protection",
		},
		{
			name:        "division with or protection",
			expr:        "sum(a) / sum(b) or 0",
			expectIssue: false,
		},
		{
			name:        "division with != 0 check",
			expr:        "sum(a{b!= 0}) / sum(b)",
			expectIssue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := checkInstrumentationPatterns(tt.expr)
			if tt.expectIssue && len(issues) == 0 {
				t.Errorf("Expected %s issue but got none", tt.issueType)
			}
			if !tt.expectIssue && len(issues) > 0 {
				t.Errorf("Expected no issues but got: %v", issues)
			}
		})
	}
}

func TestExtractMetricNames(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "simple metric",
			expr:     "up",
			expected: []string{"up"},
		},
		{
			name:     "metric with selector",
			expr:     `http_requests_total{job="api"}`,
			expected: []string{"http_requests_total"},
		},
		{
			name:     "expression with function",
			expr:     "rate(http_requests_total[5m])",
			expected: []string{"http_requests_total"},
		},
		{
			name:     "complex expression",
			expr:     "sum(rate(http_requests_total[5m])) / sum(rate(http_responses_total[5m]))",
			expected: []string{"http_requests_total", "http_responses_total"},
		},
		{
			name:     "filter out functions",
			expr:     "rate(metric[5m])",
			expected: []string{"metric"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMetricNames(tt.expr)

			// Check that all expected metrics are present
			for _, expected := range tt.expected {
				found := false
				for _, got := range result {
					if got == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected metric '%s' not found in result: %v", expected, result)
				}
			}

			// Check we didn't extract extra metrics (allowing for noise)
			if len(result) > len(tt.expected)+2 {
				t.Errorf("Got too many metrics: %v (expected around %d)", result, len(tt.expected))
			}
		})
	}
}

func TestIsPromQLKeywordNew(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"rate function", "rate", true},
		{"sum aggregation", "sum", true},
		{"by clause", "by", true},
		{"metric name", "http_requests_total", false},
		{"custom name", "myapp_metric", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPromQLKeyword(tt.input)
			if result != tt.expected {
				t.Errorf("isPromQLKeyword(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectAggregationStyle(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected AggregationStyle
	}{
		{
			name:     "postfix style - sum by",
			expr:     "sum(rate(http_requests_total[5m])) by (job)",
			expected: AggregationStylePostfix,
		},
		{
			name:     "postfix style - avg without",
			expr:     "avg(metric) without (instance)",
			expected: AggregationStylePostfix,
		},
		{
			name:     "prefix style - sum by",
			expr:     "sum by (job) (rate(http_requests_total[5m]))",
			expected: AggregationStylePrefix,
		},
		{
			name:     "prefix style - max without",
			expr:     "max without (instance) (metric)",
			expected: AggregationStylePrefix,
		},
		{
			name:     "no aggregation clause",
			expr:     "sum(rate(http_requests_total[5m]))",
			expected: AggregationStyleUnknown,
		},
		{
			name:     "simple metric",
			expr:     "up",
			expected: AggregationStyleUnknown,
		},
		{
			name:     "topk postfix",
			expr:     "topk(5, http_requests_total) by (job)",
			expected: AggregationStylePostfix,
		},
		{
			name:     "topk prefix",
			expr:     "topk by (job) (5, http_requests_total)",
			expected: AggregationStylePrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectAggregationStyle(tt.expr)
			if result != tt.expected {
				t.Errorf("detectAggregationStyle(%q) = %v, want %v", tt.expr, result, tt.expected)
			}
		})
	}
}

func TestAggregationConsistency(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectIssues bool
	}{
		{
			name: "consistent postfix style",
			content: `groups:
  - name: test
    rules:
      - alert: Test1
        expr: sum(rate(metric1[5m])) by (job)
      - alert: Test2
        expr: avg(metric2) by (instance)`,
			expectIssues: false,
		},
		{
			name: "consistent prefix style",
			content: `groups:
  - name: test
    rules:
      - alert: Test1
        expr: sum by (job) (rate(metric1[5m]))
      - alert: Test2
        expr: avg by (instance) (metric2)`,
			expectIssues: false,
		},
		{
			name: "inconsistent - mixed styles",
			content: `groups:
  - name: test
    rules:
      - alert: Test1
        expr: sum(rate(metric1[5m])) by (job)
      - alert: Test2
        expr: avg by (instance) (metric2)
      - alert: Test3
        expr: max(metric3) by (pod)`,
			expectIssues: true,
		},
		{
			name: "single expression - no consistency check",
			content: `groups:
  - name: test
    rules:
      - alert: Test1
        expr: sum(rate(metric1[5m])) by (job)`,
			expectIssues: false,
		},
		{
			name: "no aggregation clauses",
			content: `groups:
  - name: test
    rules:
      - alert: Test1
        expr: up == 0
      - alert: Test2
        expr: rate(metric[5m]) > 0.5`,
			expectIssues: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := CheckOptions{DisableLineLength: false}
			issues, _ := CheckAndFormatPromQL(tt.content, opts)

			hasConsistencyIssue := false
			for _, issue := range issues {
				if strings.Contains(issue, "Inconsistent aggregation clause positioning") {
					hasConsistencyIssue = true
					break
				}
			}

			if tt.expectIssues && !hasConsistencyIssue {
				t.Errorf("Expected consistency issue but got none. Issues: %v", issues)
			}
			if !tt.expectIssues && hasConsistencyIssue {
				t.Errorf("Expected no consistency issue but got one. Issues: %v", issues)
			}
		})
	}
}

func TestCheckRedundantAggregations(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		expectIssue bool
	}{
		{
			name:        "redundant by clause on both sides",
			expr:        "sum(rate(http_requests_total{job=\"api\",status=~\"5..\"}[5m])) by (instance) / sum(rate(http_requests_total{job=\"api\"}[5m])) by (instance)",
			expectIssue: true,
		},
		{
			name:        "aggregation only on right side (correct)",
			expr:        "sum(rate(http_requests_total{job=\"api\",status=~\"5..\"}[5m])) / sum(rate(http_requests_total{job=\"api\"}[5m])) by (instance)",
			expectIssue: false,
		},
		{
			name:        "different aggregation clauses",
			expr:        "sum(metric1) by (pod) / sum(metric2) by (instance)",
			expectIssue: false,
		},
		{
			name:        "no aggregation clauses",
			expr:        "sum(metric1) / sum(metric2)",
			expectIssue: false,
		},
		{
			name:        "aggregation only on left side",
			expr:        "sum(metric1) by (instance) / sum(metric2)",
			expectIssue: false,
		},
		{
			name:        "redundant without clause",
			expr:        "sum(metric1) without (pod) / sum(metric2) without (pod)",
			expectIssue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := checkRedundantAggregations(tt.expr)

			if tt.expectIssue && len(issues) == 0 {
				t.Errorf("Expected issue but got none")
			}
			if !tt.expectIssue && len(issues) > 0 {
				t.Errorf("Expected no issues but got: %v", issues)
			}
		})
	}
}

func TestExtractTrailingAggregation(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{
			name:     "by clause at end",
			expr:     "sum(rate(metric[5m])) by (instance)",
			expected: ") by (instance)",
		},
		{
			name:     "without clause at end",
			expr:     "avg(metric) without (pod)",
			expected: ") without (pod)",
		},
		{
			name:     "by clause with multiple labels",
			expr:     "sum(metric) by (instance, job)",
			expected: ") by (instance, job)",
		},
		{
			name:     "no aggregation clause",
			expr:     "sum(metric)",
			expected: "",
		},
		{
			name:     "aggregation in middle, not at end",
			expr:     "sum(metric) by (job) > 0.5",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTrailingAggregation(tt.expr)
			if result != tt.expected {
				t.Errorf("extractTrailingAggregation(%q) = %q, want %q", tt.expr, result, tt.expected)
			}
		})
	}
}

func TestCheckAggregationPlacement(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		expectIssue bool
	}{
		{
			name:        "aggregation on final operand only (correct)",
			expr:        "sum(metric1) / sum(metric2) by (instance)",
			expectIssue: false,
		},
		{
			name:        "aggregation on non-final operand",
			expr:        "sum(metric1) by (instance) / sum(metric2)",
			expectIssue: true,
		},
		{
			name:        "both operands have same aggregation (caught by redundant check)",
			expr:        "sum(metric1) by (instance) / sum(metric2) by (instance)",
			expectIssue: true,
		},
		{
			name:        "no aggregations",
			expr:        "sum(metric1) / sum(metric2)",
			expectIssue: false,
		},
		{
			name:        "complex expression with comparison",
			expr:        "sum(metric1) by (instance) / sum(metric2) > 0.5",
			expectIssue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := checkAggregationPlacement(tt.expr)

			if tt.expectIssue && len(issues) == 0 {
				t.Errorf("Expected issue but got none")
			}
			if !tt.expectIssue && len(issues) > 0 {
				t.Errorf("Expected no issues but got: %v", issues)
			}
		})
	}
}
