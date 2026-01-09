package alertmanager

import (
	"os"
	"testing"
	"time"
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
	// This test would require a temporary file
	// Skipping for now - would need to create a temp YAML file
	t.Skip("Requires file I/O - implement with temp files")
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
