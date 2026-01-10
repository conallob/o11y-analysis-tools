// Package main provides the autogen-promql-tests command for generating unit tests for Prometheus alerts and recording rules.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PrometheusRuleGroup represents a Prometheus rule group
type PrometheusRuleGroup struct {
	Name     string        `yaml:"name"`
	Interval string        `yaml:"interval,omitempty"`
	Rules    []PromQLRule  `yaml:"rules"`
}

// PromQLRule represents either an alert or recording rule
type PromQLRule struct {
	Alert       string            `yaml:"alert,omitempty"`
	Record      string            `yaml:"record,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// PrometheusRules represents the top-level Prometheus rules structure
type PrometheusRules struct {
	Groups []PrometheusRuleGroup `yaml:"groups"`
}

// TestFile represents a Prometheus unit test file
type TestFile struct {
	RuleFiles []string   `yaml:"rule_files"`
	Tests     []TestCase `yaml:"tests"`
}

// TestCase represents a single test case
type TestCase struct {
	Interval    string         `yaml:"interval"`
	InputSeries []InputSeries  `yaml:"input_series"`
	AlertRule   []AlertTest    `yaml:"alert_rules,omitempty"`
}

// InputSeries represents time series input data
type InputSeries struct {
	Series string `yaml:"series"`
	Values string `yaml:"values"`
}

// AlertTest represents expected alert behavior
type AlertTest struct {
	EvalTime  string            `yaml:"eval_time"`
	Alertname string            `yaml:"alertname"`
	ExpAlerts []ExpectedAlert   `yaml:"exp_alerts,omitempty"`
}

// ExpectedAlert represents an expected firing alert
type ExpectedAlert struct {
	ExpLabels      map[string]string `yaml:"exp_labels,omitempty"`
	ExpAnnotations map[string]string `yaml:"exp_annotations,omitempty"`
}

func main() {
	var (
		rulesFile = flag.String("rules", "", "path to Prometheus rules file (required)")
		testFile  = flag.String("tests", "", "path to test file (if it exists)")
		fixMode   = flag.Bool("fix", false, "generate missing tests")
		verbose   = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: autogen-promql-tests [options]\n\n")
		fmt.Fprintf(os.Stderr, "Identify alerts and rules without test coverage and optionally generate tests.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Check for untested alerts\n")
		fmt.Fprintf(os.Stderr, "  autogen-promql-tests --rules=./alerts.yml\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate tests for untested alerts\n")
		fmt.Fprintf(os.Stderr, "  autogen-promql-tests --rules=./alerts.yml --fix\n\n")
		fmt.Fprintf(os.Stderr, "  # Check against existing test file\n")
		fmt.Fprintf(os.Stderr, "  autogen-promql-tests --rules=./alerts.yml --tests=./alerts_test.yml\n")
	}

	flag.Parse()

	if *rulesFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --rules is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load rules file
	if *verbose {
		fmt.Printf("Loading rules from %s...\n", *rulesFile)
	}

	rules, err := loadRulesFile(*rulesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading rules file: %v\n", err)
		os.Exit(1)
	}

	// Extract all alerts and recording rules
	var alerts []PromQLRule
	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			if rule.Alert != "" || rule.Record != "" {
				alerts = append(alerts, rule)
			}
		}
	}

	if len(alerts) == 0 {
		fmt.Println("No alerts or recording rules found in rules file")
		os.Exit(0)
	}

	fmt.Printf("Found %d alerts/rules in %s\n\n", len(alerts), *rulesFile)

	// Load test file if specified
	var testedAlerts map[string]bool
	if *testFile != "" {
		tests, err := loadTestFile(*testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not load test file: %v\n", err)
			testedAlerts = make(map[string]bool)
		} else {
			testedAlerts = extractTestedAlerts(tests)
			if *verbose {
				fmt.Printf("Found %d tested alerts in %s\n", len(testedAlerts), *testFile)
			}
		}
	} else {
		testedAlerts = make(map[string]bool)
		// Try to auto-discover test file
		testPath := strings.TrimSuffix(*rulesFile, filepath.Ext(*rulesFile)) + "_test.yml"
		if _, err := os.Stat(testPath); err == nil {
			tests, err := loadTestFile(testPath)
			if err == nil {
				testedAlerts = extractTestedAlerts(tests)
				if *verbose {
					fmt.Printf("Auto-discovered test file: %s\n", testPath)
					fmt.Printf("Found %d tested alerts\n", len(testedAlerts))
				}
			}
		}
	}

	// Identify untested alerts
	var untestedAlerts []PromQLRule
	for _, alert := range alerts {
		name := alert.Alert
		if name == "" {
			name = alert.Record
		}
		if !testedAlerts[name] {
			untestedAlerts = append(untestedAlerts, alert)
		}
	}

	// Display results
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Test Coverage Analysis")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("Total rules/alerts: %d\n", len(alerts))
	fmt.Printf("Tested: %d\n", len(testedAlerts))
	fmt.Printf("Untested: %d\n", len(untestedAlerts))
	fmt.Println()

	if len(untestedAlerts) > 0 {
		fmt.Println("Untested alerts/rules:")
		for _, alert := range untestedAlerts {
			name := alert.Alert
			if name == "" {
				name = alert.Record
			}
			fmt.Printf("  • %s\n", name)
		}
		fmt.Println()
	}

	// Fix mode: generate tests
	switch {
	case *fixMode && len(untestedAlerts) > 0:
		outputPath := strings.TrimSuffix(*rulesFile, filepath.Ext(*rulesFile)) + "_test.yml"
		if *testFile != "" {
			outputPath = *testFile
		}

		fmt.Printf("Generating tests for %d untested alerts/rules...\n", len(untestedAlerts))
		fmt.Printf("Output file: %s\n\n", outputPath)

		testContent := generateTests(*rulesFile, untestedAlerts)

		if err := os.WriteFile(outputPath, []byte(testContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing test file: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✓ Test file generated successfully")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review generated tests and adjust input values")
		fmt.Println("  2. Add any edge case tests where indicated")
		fmt.Println("  3. Run tests with: promtool test rules " + outputPath)
	case len(untestedAlerts) > 0:
		fmt.Println("Run with --fix to generate tests for untested alerts")
		os.Exit(1)
	default:
		fmt.Println("✓ All alerts have test coverage")
	}
}

func loadRulesFile(filename string) (*PrometheusRules, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var rules PrometheusRules
	if err := yaml.Unmarshal(content, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &rules, nil
}

func loadTestFile(filename string) (*TestFile, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var tests TestFile
	if err := yaml.Unmarshal(content, &tests); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &tests, nil
}

func extractTestedAlerts(tests *TestFile) map[string]bool {
	tested := make(map[string]bool)
	for _, test := range tests.Tests {
		for _, alertRule := range test.AlertRule {
			tested[alertRule.Alertname] = true
		}
	}
	return tested
}

func generateTests(rulesFile string, alerts []PromQLRule) string {
	var sb strings.Builder

	sb.WriteString("# Auto-generated test file for: " + filepath.Base(rulesFile) + "\n")
	sb.WriteString("# Generated at: " + time.Now().Format(time.RFC3339) + "\n")
	sb.WriteString("#\n")
	sb.WriteString("# This file contains unit tests for Prometheus alerts and recording rules.\n")
	sb.WriteString("# Run with: promtool test rules " + filepath.Base(strings.TrimSuffix(rulesFile, filepath.Ext(rulesFile)) + "_test.yml") + "\n")
	sb.WriteString("#\n")
	sb.WriteString("# Test cases include:\n")
	sb.WriteString("#   1. True Positive: Alert should fire when condition is met\n")
	sb.WriteString("#   2. False Positive: Alert should NOT fire when condition is not met\n")
	sb.WriteString("#   3. Hysteresis: Test the 'for' duration threshold\n")
	sb.WriteString("#   4. Edge Cases: Add custom edge case tests as needed\n\n")

	sb.WriteString("rule_files:\n")
	sb.WriteString("  - " + filepath.Base(rulesFile) + "\n\n")

	sb.WriteString("evaluation_interval: 1m\n\n")

	sb.WriteString("tests:\n")

	for _, alert := range alerts {
		sb.WriteString(generateAlertTest(alert))
	}

	return sb.String()
}

func generateAlertTest(alert PromQLRule) string {
	var sb strings.Builder

	name := alert.Alert
	if name == "" {
		name = alert.Record
	}

	sb.WriteString("  # ═══════════════════════════════════════════════════════════\n")
	sb.WriteString("  # Tests for: " + name + "\n")
	sb.WriteString("  # ═══════════════════════════════════════════════════════════\n\n")

	// Test 1: True Positive
	sb.WriteString("  # Test 1: True Positive - Alert should fire\n")
	sb.WriteString("  - interval: 1m\n")
	sb.WriteString("    input_series:\n")
	sb.WriteString("      # TODO: Adjust these metrics to match your actual metrics\n")
	sb.WriteString(generateInputSeries(alert, true))
	sb.WriteString("\n")

	if alert.Alert != "" {
		sb.WriteString("    alert_rules:\n")
		sb.WriteString("      - alertname: " + alert.Alert + "\n")

		// Calculate eval time based on "for" duration
		forDuration := alert.For
		evalTime := "10m"
		if forDuration != "" {
			evalTime = calculateEvalTime(forDuration)
		}

		sb.WriteString("        eval_time: " + evalTime + "\n")
		sb.WriteString("        exp_alerts:\n")
		sb.WriteString("          - exp_labels:\n")

		// Add expected labels
		if len(alert.Labels) > 0 {
			for k, v := range alert.Labels {
				sb.WriteString("              " + k + ": " + v + "\n")
			}
		} else {
			sb.WriteString("              # TODO: Add expected labels\n")
		}

		sb.WriteString("            exp_annotations:\n")
		if len(alert.Annotations) > 0 {
			// Annotations may contain template variables, provide placeholders
			sb.WriteString("              # TODO: Verify these annotations match your expected output\n")
			for k := range alert.Annotations {
				sb.WriteString("              " + k + ": \"...\"  # Template: " + alert.Annotations[k] + "\n")
			}
		}
	}

	sb.WriteString("\n")

	// Test 2: False Positive / True Negative
	sb.WriteString("  # Test 2: False Positive - Alert should NOT fire\n")
	sb.WriteString("  - interval: 1m\n")
	sb.WriteString("    input_series:\n")
	sb.WriteString("      # TODO: Adjust these metrics so alert condition is NOT met\n")
	sb.WriteString(generateInputSeries(alert, false))
	sb.WriteString("\n")

	if alert.Alert != "" {
		sb.WriteString("    alert_rules:\n")
		sb.WriteString("      - alertname: " + alert.Alert + "\n")
		sb.WriteString("        eval_time: 10m\n")
		sb.WriteString("        exp_alerts: []  # Expect no alerts\n")
	}

	sb.WriteString("\n")

	// Test 3: Hysteresis check (if "for" is specified)
	if alert.For != "" && alert.Alert != "" {
		sb.WriteString("  # Test 3: Hysteresis - Test 'for' duration (" + alert.For + ")\n")
		sb.WriteString("  - interval: 1m\n")
		sb.WriteString("    input_series:\n")
		sb.WriteString("      # Condition met but not long enough to fire\n")
		sb.WriteString(generateInputSeries(alert, true))
		sb.WriteString("\n")
		sb.WriteString("    alert_rules:\n")
		sb.WriteString("      - alertname: " + alert.Alert + "\n")

		// Eval time should be less than "for" duration
		hysteresisEvalTime := calculateHysteresisEvalTime(alert.For)
		sb.WriteString("        eval_time: " + hysteresisEvalTime + "\n")
		sb.WriteString("        exp_alerts: []  # Should not fire yet (within 'for' threshold)\n\n")
	}

	// Test 4: Edge cases placeholder
	sb.WriteString("  # Test 4: Edge Cases\n")
	sb.WriteString("  # TODO: Add tests for edge cases such as:\n")
	sb.WriteString("  #   - Boundary values\n")
	sb.WriteString("  #   - Missing metrics\n")
	sb.WriteString("  #   - Label combinations\n")
	sb.WriteString("  #   - Recovery behavior\n\n")

	return sb.String()
}

func generateInputSeries(alert PromQLRule, shouldFire bool) string {
	var sb strings.Builder

	// Parse the expression to extract metric names
	// This is a simplified heuristic - in production, you'd want proper PromQL parsing
	expr := alert.Expr

	sb.WriteString("      - series: 'example_metric{job=\"test\", instance=\"localhost:9090\"}'\n")

	if shouldFire {
		sb.WriteString("        values: '0+10x10'  # TODO: Adjust to trigger alert\n")
	} else {
		sb.WriteString("        values: '0+1x10'   # TODO: Adjust to NOT trigger alert\n")
	}

	sb.WriteString("      # TODO: Add additional metrics required by: " + strings.Split(expr, "\n")[0] + "\n")

	return sb.String()
}

func calculateEvalTime(forDuration string) string {
	// Parse "for" duration and add buffer for testing
	// Simple heuristic: if "for" is 5m, eval at 10m to be safe
	d, err := time.ParseDuration(forDuration)
	if err != nil {
		return "10m"
	}

	evalDuration := d * 2
	if evalDuration < 5*time.Minute {
		evalDuration = 10 * time.Minute
	}

	return formatDuration(evalDuration)
}

func calculateHysteresisEvalTime(forDuration string) string {
	// Eval time should be less than "for" duration
	d, err := time.ParseDuration(forDuration)
	if err != nil {
		return "2m"
	}

	hysteresisDuration := d / 2
	if hysteresisDuration < 1*time.Minute {
		hysteresisDuration = 1 * time.Minute
	}

	return formatDuration(hysteresisDuration)
}

func formatDuration(d time.Duration) string {
	if d >= time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh", hours)
	}
	if d >= time.Minute {
		minutes := int(d.Minutes())
		return fmt.Sprintf("%dm", minutes)
	}
	seconds := int(d.Seconds())
	return fmt.Sprintf("%ds", seconds)
}
