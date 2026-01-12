package alertmanager

import (
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestRoundToSensibleDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{
			name:     "very short duration",
			input:    10 * time.Second,
			expected: 30 * time.Second,
		},
		{
			name:     "45 seconds rounds to 1 minute",
			input:    45 * time.Second,
			expected: 1 * time.Minute,
		},
		{
			name:     "90 seconds rounds to 2 minutes",
			input:    90 * time.Second,
			expected: 2 * time.Minute,
		},
		{
			name:     "3 minutes rounds to 5 minutes",
			input:    3 * time.Minute,
			expected: 5 * time.Minute,
		},
		{
			name:     "7 minutes rounds to 10 minutes",
			input:    7 * time.Minute,
			expected: 10 * time.Minute,
		},
		{
			name:     "20 minutes rounds to 30 minutes",
			input:    20 * time.Minute,
			expected: 30 * time.Minute,
		},
		{
			name:     "45 minutes rounds to 1 hour",
			input:    45 * time.Minute,
			expected: 1 * time.Hour,
		},
		{
			name:     "very long duration",
			input:    48 * time.Hour,
			expected: 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := roundToSensibleDuration(tt.input)
			if result != tt.expected {
				t.Errorf("roundToSensibleDuration(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAnalyzeAlert(t *testing.T) {
	analyzer := NewHysteresisAnalyzer("http://localhost:9090", false)

	tests := []struct {
		name      string
		events    []AlertEvent
		wantCount int
		wantMin   time.Duration
		wantMax   time.Duration
	}{
		{
			name:      "no events",
			events:    []AlertEvent{},
			wantCount: 0,
		},
		{
			name: "single event",
			events: []AlertEvent{
				{
					AlertName: "TestAlert",
					StartsAt:  time.Now().Add(-5 * time.Minute),
					EndsAt:    time.Now(),
					Duration:  5 * time.Minute,
				},
			},
			wantCount: 1,
			wantMin:   5 * time.Minute,
			wantMax:   5 * time.Minute,
		},
		{
			name: "multiple events with varying durations",
			events: []AlertEvent{
				{Duration: 1 * time.Minute},
				{Duration: 5 * time.Minute},
				{Duration: 10 * time.Minute},
				{Duration: 3 * time.Minute},
				{Duration: 7 * time.Minute},
			},
			wantCount: 5,
			wantMin:   1 * time.Minute,
			wantMax:   10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzer.AnalyzeAlert("TestAlert", tt.events)

			if analysis.FiringCount != tt.wantCount {
				t.Errorf("FiringCount = %d, want %d", analysis.FiringCount, tt.wantCount)
			}

			if tt.wantCount > 0 {
				if analysis.MinDuration != tt.wantMin {
					t.Errorf("MinDuration = %v, want %v", analysis.MinDuration, tt.wantMin)
				}

				if analysis.MaxDuration != tt.wantMax {
					t.Errorf("MaxDuration = %v, want %v", analysis.MaxDuration, tt.wantMax)
				}

				if analysis.AvgDuration == 0 {
					t.Error("AvgDuration should not be zero for non-empty events")
				}

				if analysis.MedianDuration == 0 {
					t.Error("MedianDuration should not be zero for non-empty events")
				}
			}
		})
	}
}

func TestAnalyzeAlertRecommendation(t *testing.T) {
	analyzer := NewHysteresisAnalyzer("http://localhost:9090", false)

	// Create events where 30% are short-lived (under 2 minutes)
	// and 70% are longer
	events := []AlertEvent{
		{Duration: 30 * time.Second}, // spurious
		{Duration: 45 * time.Second}, // spurious
		{Duration: 1 * time.Minute},  // spurious
		{Duration: 5 * time.Minute},  // legitimate
		{Duration: 10 * time.Minute}, // legitimate
		{Duration: 8 * time.Minute},  // legitimate
		{Duration: 12 * time.Minute}, // legitimate
		{Duration: 6 * time.Minute},  // legitimate
		{Duration: 7 * time.Minute},  // legitimate
		{Duration: 9 * time.Minute},  // legitimate
	}

	analysis := analyzer.AnalyzeAlert("TestAlert", events)

	// Should recommend a duration that filters out the short ones
	if analysis.RecommendedFor == 0 {
		t.Error("RecommendedFor should not be zero")
	}

	// Should have identified spurious alerts
	if analysis.SpuriousAlerts == 0 {
		t.Error("Should have identified some spurious alerts")
	}

	// Should have reasoning
	if analysis.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}

	t.Logf("Recommendation: %v", analysis.RecommendedFor)
	t.Logf("Spurious alerts: %d/%d", analysis.SpuriousAlerts, analysis.FiringCount)
	t.Logf("Reasoning: %s", analysis.Reasoning)
}

func TestNewHysteresisAnalyzer(t *testing.T) {
	url := "http://prometheus:9090"
	verbose := true

	analyzer := NewHysteresisAnalyzer(url, verbose)

	if analyzer == nil {
		t.Fatal("NewHysteresisAnalyzer returned nil")
	}

	if analyzer.prometheusURL != url {
		t.Errorf("prometheusURL = %q, want %q", analyzer.prometheusURL, url)
	}

	if analyzer.verbose != verbose {
		t.Errorf("verbose = %v, want %v", analyzer.verbose, verbose)
	}
}

func TestLoadAlertDurations(t *testing.T) {
	tmpFile := t.TempDir() + "/test-rules.yml"
	content := `groups:
  - name: test-group
    rules:
      - alert: HighErrorRate
        expr: error_rate > 0.1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: High error rate detected
      - alert: LowDiskSpace
        expr: disk_usage > 90
        for: 10m
      - record: job:error_rate:5m
        expr: rate(errors_total[5m])
      - alert: NoForDuration
        expr: cpu_usage > 80
`
	if err := writeTestFile(tmpFile, content); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	durations, err := LoadAlertDurations(tmpFile)
	if err != nil {
		t.Fatalf("LoadAlertDurations failed: %v", err)
	}

	expected := map[string]time.Duration{
		"HighErrorRate": 5 * time.Minute,
		"LowDiskSpace":  10 * time.Minute,
	}

	if len(durations) != len(expected) {
		t.Errorf("Got %d durations, want %d", len(durations), len(expected))
	}

	for name, expectedDuration := range expected {
		if got, ok := durations[name]; !ok {
			t.Errorf("Missing duration for alert %s", name)
		} else if got != expectedDuration {
			t.Errorf("Duration for %s = %v, want %v", name, got, expectedDuration)
		}
	}
}

func TestGetAlertNamesFromRules(t *testing.T) {
	// Create a temporary rules file
	tmpFile := t.TempDir() + "/test-rules.yml"
	content := `groups:
  - name: test-group
    rules:
      - alert: HighErrorRate
        expr: error_rate > 0.1
        for: 5m
      - alert: LowDiskSpace
        expr: disk_usage > 90
        for: 10m
      - alert: HighCPU
        expr: cpu_usage > 80
        for: 2m
`
	if err := writeTestFile(tmpFile, content); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	alertNames, err := GetAlertNamesFromRules(tmpFile)
	if err != nil {
		t.Fatalf("GetAlertNamesFromRules failed: %v", err)
	}

	expected := map[string]bool{
		"HighErrorRate":  true,
		"LowDiskSpace":   true,
		"HighCPU":        true,
	}

	if len(alertNames) != len(expected) {
		t.Errorf("Got %d alerts, want %d", len(alertNames), len(expected))
	}

	for _, name := range alertNames {
		if !expected[name] {
			t.Errorf("Unexpected alert name: %s", name)
		}
	}
}

func TestDeleteAlertsFromRules(t *testing.T) {
	// Create a temporary rules file
	tmpFile := t.TempDir() + "/test-rules.yml"
	content := `groups:
  - name: test-group
    rules:
      - alert: HighErrorRate
        for: 5m
      - alert: LowDiskSpace
        for: 10m
      - alert: HighCPU
        for: 2m
`
	if err := writeTestFile(tmpFile, content); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Delete one alert
	toDelete := []string{"LowDiskSpace"}
	if err := DeleteAlertsFromRules(tmpFile, toDelete); err != nil {
		t.Fatalf("DeleteAlertsFromRules failed: %v", err)
	}

	// Verify the alert was deleted
	alertNames, err := GetAlertNamesFromRules(tmpFile)
	if err != nil {
		t.Fatalf("GetAlertNamesFromRules failed: %v", err)
	}

	if len(alertNames) != 2 {
		t.Errorf("Expected 2 alerts after deletion, got %d", len(alertNames))
	}

	for _, name := range alertNames {
		if name == "LowDiskSpace" {
			t.Error("LowDiskSpace should have been deleted")
		}
	}
}

func TestDeleteMultipleAlertsFromRules(t *testing.T) {
	// Create a temporary rules file
	tmpFile := t.TempDir() + "/test-rules.yml"
	content := `groups:
  - name: test-group
    rules:
      - alert: Alert1
        for: 5m
      - alert: Alert2
        for: 10m
      - alert: Alert3
        for: 2m
      - alert: Alert4
        for: 1m
`
	if err := writeTestFile(tmpFile, content); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Delete multiple alerts
	toDelete := []string{"Alert1", "Alert3"}
	if err := DeleteAlertsFromRules(tmpFile, toDelete); err != nil {
		t.Fatalf("DeleteAlertsFromRules failed: %v", err)
	}

	// Verify only expected alerts remain
	alertNames, err := GetAlertNamesFromRules(tmpFile)
	if err != nil {
		t.Fatalf("GetAlertNamesFromRules failed: %v", err)
	}

	if len(alertNames) != 2 {
		t.Errorf("Expected 2 alerts after deletion, got %d", len(alertNames))
	}

	expected := map[string]bool{
		"Alert2": true,
		"Alert4": true,
	}

	for _, name := range alertNames {
		if !expected[name] {
			t.Errorf("Unexpected alert: %s", name)
		}
	}
}

func TestAnalyzeAlertWithPercentile(t *testing.T) {
	analyzer := NewHysteresisAnalyzer("http://localhost:9090", false)

	tests := []struct {
		name             string
		events           []AlertEvent
		targetPercentile float64
		wantCount        int
		checkRecommended bool
		minRecommended   time.Duration
		maxRecommended   time.Duration
	}{
		{
			name:             "no events",
			events:           []AlertEvent{},
			targetPercentile: 0.5,
			wantCount:        0,
		},
		{
			name: "single event at 50th percentile",
			events: []AlertEvent{
				{Duration: 5 * time.Minute},
			},
			targetPercentile: 0.5,
			wantCount:        1,
			checkRecommended: true,
			minRecommended:   5 * time.Minute,
			maxRecommended:   5 * time.Minute,
		},
		{
			name: "multiple events at 20th percentile",
			events: []AlertEvent{
				{Duration: 30 * time.Second},
				{Duration: 1 * time.Minute},
				{Duration: 2 * time.Minute},
				{Duration: 5 * time.Minute},
				{Duration: 10 * time.Minute},
			},
			targetPercentile: 0.2,
			wantCount:        5,
			checkRecommended: true,
			minRecommended:   30 * time.Second,
			maxRecommended:   2 * time.Minute,
		},
		{
			name: "multiple events at 75th percentile",
			events: []AlertEvent{
				{Duration: 1 * time.Minute},
				{Duration: 2 * time.Minute},
				{Duration: 3 * time.Minute},
				{Duration: 10 * time.Minute},
			},
			targetPercentile: 0.75,
			wantCount:        4,
			checkRecommended: true,
			minRecommended:   3 * time.Minute,
			maxRecommended:   10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := analyzer.AnalyzeAlertWithPercentile("TestAlert", tt.events, tt.targetPercentile)

			if analysis.FiringCount != tt.wantCount {
				t.Errorf("FiringCount = %d, want %d", analysis.FiringCount, tt.wantCount)
			}

			if analysis.TargetPercentile != tt.targetPercentile {
				t.Errorf("TargetPercentile = %v, want %v", analysis.TargetPercentile, tt.targetPercentile)
			}

			if tt.wantCount > 0 {
				if analysis.AlertName != "TestAlert" {
					t.Errorf("AlertName = %s, want TestAlert", analysis.AlertName)
				}

				if analysis.MinDuration == 0 {
					t.Error("MinDuration should not be zero for non-empty events")
				}

				if analysis.MaxDuration == 0 {
					t.Error("MaxDuration should not be zero for non-empty events")
				}

				if analysis.AvgDuration == 0 {
					t.Error("AvgDuration should not be zero for non-empty events")
				}

				if analysis.MedianDuration == 0 {
					t.Error("MedianDuration should not be zero for non-empty events")
				}

				if tt.checkRecommended {
					if analysis.RecommendedFor < tt.minRecommended {
						t.Errorf("RecommendedFor = %v, should be >= %v", analysis.RecommendedFor, tt.minRecommended)
					}
					if analysis.RecommendedFor > tt.maxRecommended {
						t.Errorf("RecommendedFor = %v, should be <= %v", analysis.RecommendedFor, tt.maxRecommended)
					}
				}

				// Check that spurious alerts count makes sense
				if analysis.SpuriousAlerts < 0 || analysis.SpuriousAlerts > tt.wantCount {
					t.Errorf("SpuriousAlerts = %d, should be between 0 and %d", analysis.SpuriousAlerts, tt.wantCount)
				}

				if analysis.PreventedAlerts != analysis.SpuriousAlerts {
					t.Errorf("PreventedAlerts = %d, should equal SpuriousAlerts = %d", analysis.PreventedAlerts, analysis.SpuriousAlerts)
				}
			}
		})
	}
}

func TestUpdateAlertDurations(t *testing.T) {
	// Create a temporary rules file with complete rule structure
	tmpFile := t.TempDir() + "/test-rules.yml"
	content := `groups:
  - name: test-group
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: rate(errors_total[5m]) > 0.1
        for: 1m
        labels:
          severity: critical
          team: backend
        annotations:
          summary: High error rate detected
          description: Error rate is above threshold
      - alert: LowDiskSpace
        expr: disk_usage_percent > 90
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: Disk space running low
      - record: job:error_rate:5m
        expr: rate(errors_total[5m])
`
	if err := writeTestFile(tmpFile, content); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Update durations
	recommendations := map[string]time.Duration{
		"HighErrorRate": 5 * time.Minute,
		"LowDiskSpace":  15 * time.Minute,
	}

	if err := UpdateAlertDurations(tmpFile, recommendations); err != nil {
		t.Fatalf("UpdateAlertDurations failed: %v", err)
	}

	// Verify the durations were updated
	durations, err := LoadAlertDurations(tmpFile)
	if err != nil {
		t.Fatalf("LoadAlertDurations failed: %v", err)
	}

	for alertName, expectedDuration := range recommendations {
		if got, ok := durations[alertName]; !ok {
			t.Errorf("Alert %s not found after update", alertName)
		} else if got != expectedDuration {
			t.Errorf("Duration for %s = %v, want %v", alertName, got, expectedDuration)
		}
	}

	// Verify that other fields were preserved
	updatedContent, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	var rules PrometheusRules
	if err := yaml.Unmarshal(updatedContent, &rules); err != nil {
		t.Fatalf("Failed to parse updated YAML: %v", err)
	}

	// Check that critical fields are preserved
	for _, group := range rules.Groups {
		if group.Name != "test-group" {
			t.Errorf("Group name changed to %s", group.Name)
		}
		if group.Interval != "30s" {
			t.Errorf("Group interval changed to %s", group.Interval)
		}

		for _, rule := range group.Rules {
			if rule.Alert == "HighErrorRate" {
				if rule.Expr != "rate(errors_total[5m]) > 0.1" {
					t.Errorf("Expr was lost or changed: %s", rule.Expr)
				}
				if rule.Labels["severity"] != "critical" {
					t.Errorf("Labels were lost or changed")
				}
				if rule.Labels["team"] != "backend" {
					t.Errorf("Labels were lost or changed")
				}
				if rule.Annotations["summary"] != "High error rate detected" {
					t.Errorf("Annotations were lost or changed")
				}
				if rule.Annotations["description"] != "Error rate is above threshold" {
					t.Errorf("Annotations were lost or changed")
				}
			}
			if rule.Record == "job:error_rate:5m" {
				if rule.Expr != "rate(errors_total[5m])" {
					t.Errorf("Recording rule expr was lost or changed: %s", rule.Expr)
				}
			}
		}
	}
}

func TestFormatPrometheusDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "30 seconds",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "1 minute",
			duration: 1 * time.Minute,
			expected: "1m",
		},
		{
			name:     "5 minutes",
			duration: 5 * time.Minute,
			expected: "5m",
		},
		{
			name:     "1 hour",
			duration: 1 * time.Hour,
			expected: "1h",
		},
		{
			name:     "2 hours",
			duration: 2 * time.Hour,
			expected: "2h",
		},
		{
			name:     "1 day",
			duration: 24 * time.Hour,
			expected: "1d",
		},
		{
			name:     "90 seconds (not evenly divisible)",
			duration: 90 * time.Second,
			expected: "90s",
		},
		{
			name:     "90 minutes (1.5 hours)",
			duration: 90 * time.Minute,
			expected: "90m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPrometheusDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatPrometheusDuration(%v) = %s, want %s", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestGetSensitivityNote(t *testing.T) {
	tests := []struct {
		name       string
		percentile float64
		expected   string
	}{
		{
			name:       "very sensitive (0.1)",
			percentile: 0.1,
			expected:   "very sensitive, may catch transient issues",
		},
		{
			name:       "more sensitive (0.3)",
			percentile: 0.3,
			expected:   "more sensitive, may catch transient issues",
		},
		{
			name:       "balanced (0.5)",
			percentile: 0.5,
			expected:   "balanced sensitivity",
		},
		{
			name:       "more robust (0.65)",
			percentile: 0.65,
			expected:   "more robust, ignores transient issues",
		},
		{
			name:       "very robust (0.8)",
			percentile: 0.8,
			expected:   "very robust, ignores transient issues",
		},
		{
			name:       "edge case - 0",
			percentile: 0,
			expected:   "very sensitive, may catch transient issues",
		},
		{
			name:       "edge case - 1.0",
			percentile: 1.0,
			expected:   "very robust, ignores transient issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSensitivityNote(tt.percentile)
			if result != tt.expected {
				t.Errorf("getSensitivityNote(%v) = %s, want %s", tt.percentile, result, tt.expected)
			}
		})
	}
}

// writeTestFile is a helper function to write test files
func writeTestFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			}
		}
	}()

	_, err = file.WriteString(content)
	return err
}
