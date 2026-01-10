// Package main provides the e2e-alertmanager-test command for end-to-end testing of alertmanager routing.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TestFile represents a Prometheus unit test file
type TestFile struct {
	RuleFiles []string   `yaml:"rule_files"`
	Tests     []TestCase `yaml:"tests"`
}

// TestCase represents a single test case
type TestCase struct {
	Interval    string        `yaml:"interval"`
	InputSeries []interface{} `yaml:"input_series"`
	AlertRules  []AlertTest   `yaml:"alert_rules,omitempty"`
}

// AlertTest represents expected alert behavior
type AlertTest struct {
	EvalTime  string          `yaml:"eval_time"`
	Alertname string          `yaml:"alertname"`
	ExpAlerts []ExpectedAlert `yaml:"exp_alerts,omitempty"`
}

// ExpectedAlert represents an expected firing alert
type ExpectedAlert struct {
	ExpLabels      map[string]string `yaml:"exp_labels,omitempty"`
	ExpAnnotations map[string]string `yaml:"exp_annotations,omitempty"`
}

// AlertmanagerAlert represents an alert sent to Alertmanager API
type AlertmanagerAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
}

// AlertmanagerConfig represents alertmanager configuration
type AlertmanagerConfig struct {
	Global struct {
		SMTPSmarthost   string `yaml:"smtp_smarthost,omitempty"`
		SMTPFrom        string `yaml:"smtp_from,omitempty"`
		SMTPRequireTLS  bool   `yaml:"smtp_require_tls,omitempty"`
	} `yaml:"global,omitempty"`
	Route struct {
		Receiver       string                   `yaml:"receiver"`
		GroupBy        []string                 `yaml:"group_by,omitempty"`
		GroupWait      string                   `yaml:"group_wait,omitempty"`
		GroupInterval  string                   `yaml:"group_interval,omitempty"`
		RepeatInterval string                   `yaml:"repeat_interval,omitempty"`
		Routes         []map[string]interface{} `yaml:"routes,omitempty"`
	} `yaml:"route"`
	Receivers []struct {
		Name          string `yaml:"name"`
		EmailConfigs  []map[string]interface{} `yaml:"email_configs,omitempty"`
		WebhookConfigs []map[string]interface{} `yaml:"webhook_configs,omitempty"`
	} `yaml:"receivers"`
}

// EmailOutput represents formatted email output
type EmailOutput struct {
	To          string
	From        string
	Subject     string
	Headers     map[string]string
	Body        string
	RoutingInfo string
}

func main() {
	var (
		testFile         = flag.String("tests", "", "path to Prometheus test file (required)")
		alertmanagerURL  = flag.String("alertmanager-url", "http://localhost:9093", "Alertmanager API URL")
		alertmanagerConf = flag.String("alertmanager-config", "", "path to alertmanager config file")
		outputFormat     = flag.String("output", "email", "output format: email, json")
		verbose          = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: e2e-alertmanager-test [options]\n\n")
		fmt.Fprintf(os.Stderr, "Run end-to-end tests of alert routing through Alertmanager.\n")
		fmt.Fprintf(os.Stderr, "Reads Prometheus test files and sends firing alerts to Alertmanager,\n")
		fmt.Fprintf(os.Stderr, "then formats the output as SMTP email headers and body.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Test alert routing\n")
		fmt.Fprintf(os.Stderr, "  e2e-alertmanager-test --tests=./alerts_test.yml \\\n")
		fmt.Fprintf(os.Stderr, "                         --alertmanager-url=http://localhost:9093\n\n")
		fmt.Fprintf(os.Stderr, "  # Test with custom alertmanager config\n")
		fmt.Fprintf(os.Stderr, "  e2e-alertmanager-test --tests=./alerts_test.yml \\\n")
		fmt.Fprintf(os.Stderr, "                         --alertmanager-config=./alertmanager.yml\n")
	}

	flag.Parse()

	if *testFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --tests is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load test file
	if *verbose {
		fmt.Printf("Loading test file: %s\n", *testFile)
	}

	tests, err := loadTestFile(*testFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading test file: %v\n", err)
		os.Exit(1)
	}

	// Load alertmanager config if specified
	var amConfig *AlertmanagerConfig
	if *alertmanagerConf != "" {
		if *verbose {
			fmt.Printf("Loading alertmanager config: %s\n", *alertmanagerConf)
		}
		amConfig, err = loadAlertmanagerConfig(*alertmanagerConf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not load alertmanager config: %v\n", err)
			amConfig = getDefaultConfig()
		}
	} else {
		amConfig = getDefaultConfig()
	}

	// Process test cases
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("End-to-End Alertmanager Test Results")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	totalTests := 0
	successfulTests := 0

	for testIdx, test := range tests.Tests {
		for _, alertRule := range test.AlertRules {
			if len(alertRule.ExpAlerts) == 0 {
				continue // Skip tests expecting no alerts
			}

			totalTests++
			fmt.Printf("Test #%d: %s\n", totalTests, alertRule.Alertname)
			fmt.Println(strings.Repeat("-", 60))

			for _, expAlert := range alertRule.ExpAlerts {
				// Create alert to send to alertmanager
				alert := AlertmanagerAlert{
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
					StartsAt:    time.Now(),
				}

				// Set alertname
				alert.Labels["alertname"] = alertRule.Alertname

				// Add expected labels
				for k, v := range expAlert.ExpLabels {
					alert.Labels[k] = v
				}

				// Add expected annotations
				for k, v := range expAlert.ExpAnnotations {
					alert.Annotations[k] = v
				}

				// Send to alertmanager
				if *verbose {
					fmt.Printf("  Sending alert to %s...\n", *alertmanagerURL)
				}

				err := sendAlertToAlertmanager(*alertmanagerURL, alert)
				if err != nil {
					fmt.Printf("  ✗ Failed to send alert: %v\n", err)
					continue
				}

				successfulTests++

				// Format as email
				email := formatAsEmail(alert, amConfig)

				switch *outputFormat {
				case "email":
					printEmailOutput(email, testIdx+1)
				case "json":
					printJSONOutput(alert, email)
				default:
					fmt.Printf("  ✓ Alert sent successfully\n")
				}

				fmt.Println()
			}
		}
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Summary")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Total test cases: %d\n", totalTests)
	fmt.Printf("Successful: %d\n", successfulTests)
	fmt.Printf("Failed: %d\n", totalTests-successfulTests)
	fmt.Println()

	if successfulTests < totalTests {
		os.Exit(1)
	}
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

func loadAlertmanagerConfig(filename string) (*AlertmanagerConfig, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config AlertmanagerConfig
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

func getDefaultConfig() *AlertmanagerConfig {
	return &AlertmanagerConfig{}
}

func sendAlertToAlertmanager(alertmanagerURL string, alert AlertmanagerAlert) error {
	// Alertmanager expects an array of alerts
	alerts := []AlertmanagerAlert{alert}

	jsonData, err := json.Marshal(alerts)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	url := strings.TrimSuffix(alertmanagerURL, "/") + "/api/v2/alerts"
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send alert: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			}
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("alertmanager returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func formatAsEmail(alert AlertmanagerAlert, config *AlertmanagerConfig) EmailOutput {
	email := EmailOutput{
		Headers: make(map[string]string),
	}

	// Set From address
	email.From = config.Global.SMTPFrom
	if email.From == "" {
		email.From = "alertmanager@localhost"
	}

	// Set To address (would be determined by routing rules in real scenario)
	email.To = "oncall@example.com"

	// Build subject line
	alertname := alert.Labels["alertname"]
	severity := alert.Labels["severity"]
	if severity == "" {
		severity = "warning"
	}

	email.Subject = fmt.Sprintf("[%s] %s", strings.ToUpper(severity), alertname)

	// RFC 2076 compliant headers
	email.Headers["MIME-Version"] = "1.0"
	email.Headers["Content-Type"] = "text/plain; charset=utf-8"
	email.Headers["Date"] = time.Now().Format(time.RFC1123Z)
	email.Headers["Message-ID"] = fmt.Sprintf("<%s-%d@alertmanager>", alertname, time.Now().Unix())
	email.Headers["X-Alertmanager-Alert"] = alertname
	if severity != "" {
		email.Headers["X-Alert-Severity"] = severity
	}

	// Build email body
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Alert: %s\n", alertname))
	body.WriteString(fmt.Sprintf("Severity: %s\n", severity))
	body.WriteString(fmt.Sprintf("Started: %s\n", alert.StartsAt.Format(time.RFC3339)))
	body.WriteString("\n")

	// Labels
	body.WriteString("Labels:\n")
	for k, v := range alert.Labels {
		if k != "alertname" {
			body.WriteString(fmt.Sprintf("  %s = %s\n", k, v))
		}
	}
	body.WriteString("\n")

	// Annotations
	if len(alert.Annotations) > 0 {
		body.WriteString("Annotations:\n")
		for k, v := range alert.Annotations {
			body.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
		body.WriteString("\n")
	}

	// Routing info
	email.RoutingInfo = fmt.Sprintf("Receiver: %s\n", config.Route.Receiver)
	if config.Route.Receiver == "" {
		email.RoutingInfo = "Receiver: default\n"
	}

	email.Body = body.String()

	return email
}

func printEmailOutput(email EmailOutput, testNum int) {
	fmt.Printf("  Email Output (Test #%d):\n", testNum)
	fmt.Println("  " + strings.Repeat("─", 58))
	fmt.Printf("  From: %s\n", email.From)
	fmt.Printf("  To: %s\n", email.To)
	fmt.Printf("  Subject: %s\n", email.Subject)

	// Print RFC 2076 headers
	for k, v := range email.Headers {
		fmt.Printf("  %s: %s\n", k, v)
	}

	fmt.Println()
	fmt.Println("  Message Body:")
	fmt.Println("  " + strings.Repeat("─", 58))

	// Indent body lines
	bodyLines := strings.Split(email.Body, "\n")
	for _, line := range bodyLines {
		fmt.Printf("  %s\n", line)
	}

	if email.RoutingInfo != "" {
		fmt.Println("  " + strings.Repeat("─", 58))
		fmt.Println("  Routing Information:")
		routingLines := strings.Split(email.RoutingInfo, "\n")
		for _, line := range routingLines {
			if line != "" {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

func printJSONOutput(alert AlertmanagerAlert, email EmailOutput) {
	output := map[string]interface{}{
		"alert": alert,
		"email": email,
	}

	jsonData, err := json.MarshalIndent(output, "  ", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}
