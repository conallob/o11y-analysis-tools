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

func TestCheckAlertLabels(t *testing.T) {
	content := `
groups:
  - name: test-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(errors_total[5m]) > 0.05
        for: 5m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "High error rate"

      - alert: MissingLabels
        expr: rate(errors_total[5m]) > 0.1
        labels:
          severity: critical
        annotations:
          summary: "Missing team label"

      - alert: NoLabelsSection
        expr: up == 0
        annotations:
          summary: "No labels section at all"
`

	requiredLabels := []string{"severity", "team"}
	violations := CheckAlertLabels(content, requiredLabels)

	// Should find 3 alerts total
	if len(violations) != 2 {
		t.Errorf("Expected 2 violations, got %d", len(violations))
	}

	// First alert (HighErrorRate) should have no missing labels
	// Second alert (MissingLabels) should be missing 'team'
	foundMissingTeam := false
	for _, v := range violations {
		if v.AlertName == "MissingLabels" {
			if len(v.MissingLabels) != 1 || v.MissingLabels[0] != "team" {
				t.Errorf("Alert 'MissingLabels' should be missing 'team', got %v", v.MissingLabels)
			}
			foundMissingTeam = true
		}
	}

	if !foundMissingTeam {
		t.Errorf("Did not find violation for 'MissingLabels' alert")
	}

	// Third alert (NoLabelsSection) should be missing both labels
	foundNoLabels := false
	for _, v := range violations {
		if v.AlertName == "NoLabelsSection" {
			if len(v.MissingLabels) != 2 {
				t.Errorf("Alert 'NoLabelsSection' should be missing both labels, got %v", v.MissingLabels)
			}
			foundNoLabels = true
		}
	}

	if !foundNoLabels {
		t.Errorf("Did not find violation for 'NoLabelsSection' alert")
	}
}

func TestCheckAlertLabelsWithCommonLabels(t *testing.T) {
	content := `
groups:
  - name: infrastructure-alerts
    rules:
      - alert: DatabaseDown
        expr: up{job="postgres"} == 0
        labels:
          severity: critical
          grafana_url: "https://grafana.example.com/d/postgres"
          runbook: "https://runbook.example.com/postgres-down"
        annotations:
          summary: "Database is down"
`

	requiredLabels := []string{"severity", "grafana_url", "runbook"}
	violations := CheckAlertLabels(content, requiredLabels)

	if len(violations) != 0 {
		t.Errorf("Expected no violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckAlertLabelsWithLocationLabel(t *testing.T) {
	content := `
groups:
  - name: regional-alerts
    rules:
      - alert: HighLatency
        expr: http_request_duration_seconds > 1
        labels:
          severity: warning
          location: us-east-1
        annotations:
          summary: "High latency detected"
`

	requiredLabels := []string{"severity", "location"}
	violations := CheckAlertLabels(content, requiredLabels)

	if len(violations) != 0 {
		t.Errorf("Expected no violations for alert with location label, got %d: %v", len(violations), violations)
	}
}
