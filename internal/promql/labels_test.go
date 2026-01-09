package promql

import (
	"testing"
)

func TestExtractLabelsFromExpression(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "simple label matcher",
			expr:     `up{job="api"}`,
			expected: []string{"job"},
		},
		{
			name:     "multiple labels",
			expr:     `http_requests_total{job="api",instance="localhost",status="200"}`,
			expected: []string{"job", "instance", "status"},
		},
		{
			name:     "regex matcher",
			expr:     `metric{job=~"api.*",namespace!="default"}`,
			expected: []string{"job", "namespace"},
		},
		{
			name:     "by clause",
			expr:     `sum(metric) by (job, instance)`,
			expected: []string{"job", "instance"},
		},
		{
			name:     "without clause",
			expr:     `sum(metric) without (internal_label)`,
			expected: []string{"internal_label"},
		},
		{
			name:     "no labels",
			expr:     `up`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLabelsFromExpression(tt.expr)

			// Check that all expected labels are present
			for _, expectedLabel := range tt.expected {
				found := false
				for _, resultLabel := range result {
					if resultLabel == expectedLabel {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected label %q not found in result %v", expectedLabel, result)
				}
			}

			// Check no extra labels
			if len(result) > len(tt.expected) {
				t.Errorf("Got more labels than expected: %v vs %v", result, tt.expected)
			}
		})
	}
}

func TestCheckLabelsInExpression(t *testing.T) {
	tests := []struct {
		name           string
		expr           string
		requiredLabels []string
		expectMissing  []string
	}{
		{
			name:           "all required labels present",
			expr:           `up{job="api",namespace="prod"}`,
			requiredLabels: []string{"job", "namespace"},
			expectMissing:  []string{},
		},
		{
			name:           "missing one label",
			expr:           `up{job="api"}`,
			requiredLabels: []string{"job", "namespace"},
			expectMissing:  []string{"namespace"},
		},
		{
			name:           "missing all labels",
			expr:           `up`,
			requiredLabels: []string{"job", "namespace"},
			expectMissing:  []string{"job", "namespace"},
		},
		{
			name:           "label in by clause counts",
			expr:           `sum(metric) by (job)`,
			requiredLabels: []string{"job"},
			expectMissing:  []string{},
		},
		{
			name:           "complex expression with all labels",
			expr:           `rate(metric{job="api"}[5m]) and on(instance) other{namespace="prod"}`,
			requiredLabels: []string{"job", "namespace"},
			expectMissing:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkLabelsInExpression(tt.expr, tt.requiredLabels)

			if len(result) != len(tt.expectMissing) {
				t.Errorf("Expected %d missing labels, got %d: %v", len(tt.expectMissing), len(result), result)
				return
			}

			for _, expected := range tt.expectMissing {
				found := false
				for _, actual := range result {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected missing label %q not found in result %v", expected, result)
				}
			}
		})
	}
}

func TestCheckRequiredLabels(t *testing.T) {
	content := `
groups:
  - name: test
    rules:
      - alert: Test1
        expr: up{job="api"}
      - alert: Test2
        expr: rate(metric[5m])
      - alert: Test3
        expr: sum(metric{job="api",namespace="prod"}) by (instance)
`

	requiredLabels := []string{"job", "namespace"}
	violations := CheckRequiredLabels(content, requiredLabels)

	// Should find 3 expressions total
	if len(violations) != 3 {
		t.Errorf("Expected 3 violations (one per expression), got %d", len(violations))
	}

	// First expression missing namespace
	if len(violations[0].MissingLabels) != 1 || violations[0].MissingLabels[0] != "namespace" {
		t.Errorf("First expression should be missing 'namespace', got %v", violations[0].MissingLabels)
	}

	// Second expression missing both
	if len(violations[1].MissingLabels) != 2 {
		t.Errorf("Second expression should be missing both labels, got %v", violations[1].MissingLabels)
	}

	// Third expression has all labels
	if len(violations[2].MissingLabels) != 0 {
		t.Errorf("Third expression should have no missing labels, got %v", violations[2].MissingLabels)
	}
}

func TestIsPromQLKeyword(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"and", true},
		{"or", true},
		{"unless", true},
		{"by", true},
		{"without", true},
		{"job", false},
		{"instance", false},
		{"metric_name", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isPromQLKeyword(tt.input)
			if result != tt.expected {
				t.Errorf("isPromQLKeyword(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
