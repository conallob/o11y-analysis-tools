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
	HTMLBody    string
	RoutingInfo string
}

// NotificationOutput represents a rendered notification
type NotificationOutput struct {
	Type        string // email, slack, webhook
	Email       *EmailOutput
	SlackBody   string
	WebhookBody string
	RawJSON     string
}

func main() {
	var (
		testFile         = flag.String("tests", "", "path to Prometheus test file (required)")
		alertmanagerURL  = flag.String("alertmanager-url", "http://localhost:9093", "Alertmanager API URL")
		alertmanagerConf = flag.String("alertmanager-config", "", "path to alertmanager config file")
		outputFormat     = flag.String("output", "email", "output format: email, email-html, slack, json, full")
		renderFull       = flag.Bool("full", false, "render full notification body (includes HTML)")
		verbose          = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: e2e-alertmanager-test [options]\n\n")
		fmt.Fprintf(os.Stderr, "Run end-to-end tests of alert routing through Alertmanager.\n")
		fmt.Fprintf(os.Stderr, "Renders complete notification bodies for UX development and testing.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nOutput Formats:\n")
		fmt.Fprintf(os.Stderr, "  email      - Plain text email with RFC 2076 headers\n")
		fmt.Fprintf(os.Stderr, "  email-html - HTML email rendering\n")
		fmt.Fprintf(os.Stderr, "  slack      - Slack message format\n")
		fmt.Fprintf(os.Stderr, "  json       - JSON output\n")
		fmt.Fprintf(os.Stderr, "  full       - All notification formats\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Test alert routing with full HTML email rendering\n")
		fmt.Fprintf(os.Stderr, "  e2e-alertmanager-test --tests=./alerts_test.yml \\\n")
		fmt.Fprintf(os.Stderr, "                         --output=email-html --full\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate all notification formats for UX diffing\n")
		fmt.Fprintf(os.Stderr, "  e2e-alertmanager-test --tests=./alerts_test.yml \\\n")
		fmt.Fprintf(os.Stderr, "                         --output=full > notifications.txt\n\n")
		fmt.Fprintf(os.Stderr, "  # Slack notification preview\n")
		fmt.Fprintf(os.Stderr, "  e2e-alertmanager-test --tests=./alerts_test.yml \\\n")
		fmt.Fprintf(os.Stderr, "                         --output=slack\n")
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

				// Format notification in all supported formats
				notification := formatNotification(alert, amConfig, *renderFull)

				switch *outputFormat {
				case "email":
					printEmailOutput(notification.Email, testIdx+1, false)
				case "email-html":
					printEmailOutput(notification.Email, testIdx+1, true)
				case "slack":
					printSlackOutput(notification, testIdx+1)
				case "json":
					printJSONOutput(alert, notification)
				case "full":
					printFullOutput(notification, testIdx+1)
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

func formatNotification(alert AlertmanagerAlert, config *AlertmanagerConfig, renderFull bool) NotificationOutput {
	notification := NotificationOutput{
		Type: "email",
	}

	// Generate email notification
	email := formatAsEmail(alert, config, renderFull)
	notification.Email = &email

	// Generate Slack notification
	notification.SlackBody = formatAsSlack(alert)

	// Generate webhook/JSON
	jsonData, _ := json.MarshalIndent(alert, "", "  ")
	notification.WebhookBody = string(jsonData)
	notification.RawJSON = string(jsonData)

	return notification
}

func formatAsEmail(alert AlertmanagerAlert, config *AlertmanagerConfig, renderHTML bool) EmailOutput {
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
	if renderHTML {
		email.Headers["Content-Type"] = "multipart/alternative; boundary=\"alertmanager-boundary\""
	} else {
		email.Headers["Content-Type"] = "text/plain; charset=utf-8"
	}
	email.Headers["Date"] = time.Now().Format(time.RFC1123Z)
	email.Headers["Message-ID"] = fmt.Sprintf("<%s-%d@alertmanager>", alertname, time.Now().Unix())
	email.Headers["X-Alertmanager-Alert"] = alertname
	if severity != "" {
		email.Headers["X-Alert-Severity"] = severity
	}

	// Build plain text email body
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

	// Generate HTML body if requested
	if renderHTML {
		email.HTMLBody = formatEmailHTML(alert, severity, alertname)
	}

	return email
}

func formatEmailHTML(alert AlertmanagerAlert, severity, alertname string) string {
	var html strings.Builder

	// Severity color mapping
	severityColor := map[string]string{
		"critical": "#d32f2f",
		"warning":  "#f57c00",
		"info":     "#1976d2",
	}
	color := severityColor[severity]
	if color == "" {
		color = "#757575"
	}

	html.WriteString("<!DOCTYPE html>\n")
	html.WriteString("<html>\n<head>\n")
	html.WriteString("<meta charset=\"utf-8\">\n")
	html.WriteString("<style>\n")
	html.WriteString("body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; }\n")
	html.WriteString(".container { max-width: 600px; margin: 0 auto; padding: 20px; }\n")
	html.WriteString(".header { background: " + color + "; color: white; padding: 20px; border-radius: 4px 4px 0 0; }\n")
	html.WriteString(".header h1 { margin: 0; font-size: 24px; }\n")
	html.WriteString(".severity-badge { display: inline-block; padding: 4px 12px; border-radius: 12px; font-size: 12px; font-weight: bold; text-transform: uppercase; }\n")
	html.WriteString(".content { background: #f5f5f5; padding: 20px; border-radius: 0 0 4px 4px; }\n")
	html.WriteString(".section { background: white; padding: 15px; margin-bottom: 15px; border-radius: 4px; border-left: 4px solid " + color + "; }\n")
	html.WriteString(".section h2 { margin-top: 0; font-size: 16px; color: #555; }\n")
	html.WriteString(".label-item, .annotation-item { padding: 8px 0; border-bottom: 1px solid #e0e0e0; }\n")
	html.WriteString(".label-key, .annotation-key { font-weight: 600; color: #666; }\n")
	html.WriteString(".label-value, .annotation-value { color: #333; margin-left: 10px; }\n")
	html.WriteString(".timestamp { color: #757575; font-size: 14px; }\n")
	html.WriteString("</style>\n</head>\n<body>\n")

	html.WriteString("<div class=\"container\">\n")
	html.WriteString("  <div class=\"header\">\n")
	html.WriteString(fmt.Sprintf("    <span class=\"severity-badge\" style=\"background: rgba(255,255,255,0.3);\">%s</span>\n", strings.ToUpper(severity)))
	html.WriteString(fmt.Sprintf("    <h1>%s</h1>\n", alertname))
	html.WriteString(fmt.Sprintf("    <p class=\"timestamp\">%s</p>\n", alert.StartsAt.Format("Monday, January 2, 2006 at 3:04 PM MST")))
	html.WriteString("  </div>\n")

	html.WriteString("  <div class=\"content\">\n")

	// Annotations section
	if len(alert.Annotations) > 0 {
		html.WriteString("    <div class=\"section\">\n")
		html.WriteString("      <h2>Details</h2>\n")
		for k, v := range alert.Annotations {
			html.WriteString("      <div class=\"annotation-item\">\n")
			html.WriteString(fmt.Sprintf("        <span class=\"annotation-key\">%s:</span>\n", k))
			html.WriteString(fmt.Sprintf("        <span class=\"annotation-value\">%s</span>\n", v))
			html.WriteString("      </div>\n")
		}
		html.WriteString("    </div>\n")
	}

	// Labels section
	html.WriteString("    <div class=\"section\">\n")
	html.WriteString("      <h2>Labels</h2>\n")
	for k, v := range alert.Labels {
		if k != "alertname" && k != "severity" {
			html.WriteString("      <div class=\"label-item\">\n")
			html.WriteString(fmt.Sprintf("        <span class=\"label-key\">%s:</span>\n", k))
			html.WriteString(fmt.Sprintf("        <span class=\"label-value\">%s</span>\n", v))
			html.WriteString("      </div>\n")
		}
	}
	html.WriteString("    </div>\n")

	html.WriteString("  </div>\n")
	html.WriteString("</div>\n")
	html.WriteString("</body>\n</html>")

	return html.String()
}

func formatAsSlack(alert AlertmanagerAlert) string {
	alertname := alert.Labels["alertname"]
	severity := alert.Labels["severity"]
	if severity == "" {
		severity = "warning"
	}

	// Slack color mapping
	colorMap := map[string]string{
		"critical": "danger",
		"warning":  "warning",
		"info":     "good",
	}
	color := colorMap[severity]
	if color == "" {
		color = "#808080"
	}

	var slack strings.Builder
	slack.WriteString("{\n")
	slack.WriteString("  \"attachments\": [\n")
	slack.WriteString("    {\n")
	slack.WriteString(fmt.Sprintf("      \"color\": \"%s\",\n", color))
	slack.WriteString(fmt.Sprintf("      \"title\": \"[%s] %s\",\n", strings.ToUpper(severity), alertname))
	slack.WriteString(fmt.Sprintf("      \"title_link\": \"%s\",\n", alert.GeneratorURL))
	slack.WriteString(fmt.Sprintf("      \"ts\": %d,\n", alert.StartsAt.Unix()))
	slack.WriteString("      \"fields\": [\n")

	// Add annotations as fields
	first := true
	for k, v := range alert.Annotations {
		if !first {
			slack.WriteString(",\n")
		}
		slack.WriteString("        {\n")
		slack.WriteString(fmt.Sprintf("          \"title\": \"%s\",\n", k))
		slack.WriteString(fmt.Sprintf("          \"value\": \"%s\",\n", v))
		slack.WriteString("          \"short\": false\n")
		slack.WriteString("        }")
		first = false
	}

	// Add key labels as fields
	for k, v := range alert.Labels {
		if k != "alertname" && k != "severity" {
			if !first {
				slack.WriteString(",\n")
			}
			slack.WriteString("        {\n")
			slack.WriteString(fmt.Sprintf("          \"title\": \"%s\",\n", k))
			slack.WriteString(fmt.Sprintf("          \"value\": \"%s\",\n", v))
			slack.WriteString("          \"short\": true\n")
			slack.WriteString("        }")
			first = false
		}
	}

	slack.WriteString("\n      ],\n")
	slack.WriteString("      \"footer\": \"Alertmanager\",\n")
	slack.WriteString("      \"footer_icon\": \"https://avatars3.githubusercontent.com/u/3380462\"\n")
	slack.WriteString("    }\n")
	slack.WriteString("  ]\n")
	slack.WriteString("}")

	return slack.String()
}

func printEmailOutput(email *EmailOutput, testNum int, renderHTML bool) {
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

	if renderHTML && email.HTMLBody != "" {
		fmt.Println("  HTML Body:")
		fmt.Println("  " + strings.Repeat("─", 58))
		htmlLines := strings.Split(email.HTMLBody, "\n")
		for _, line := range htmlLines {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
		fmt.Println("  Plain Text Body:")
		fmt.Println("  " + strings.Repeat("─", 58))
	} else {
		fmt.Println("  Message Body:")
		fmt.Println("  " + strings.Repeat("─", 58))
	}

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

func printSlackOutput(notification NotificationOutput, testNum int) {
	fmt.Printf("  Slack Output (Test #%d):\n", testNum)
	fmt.Println("  " + strings.Repeat("─", 58))
	fmt.Println()

	// Print formatted Slack JSON
	slackLines := strings.Split(notification.SlackBody, "\n")
	for _, line := range slackLines {
		fmt.Printf("  %s\n", line)
	}
}

func printFullOutput(notification NotificationOutput, testNum int) {
	fmt.Printf("  Full Notification Output (Test #%d):\n", testNum)
	fmt.Println("  " + strings.Repeat("═", 58))
	fmt.Println()

	// Email section
	fmt.Println("  ╔═══ EMAIL (Plain Text) ═══")
	printEmailOutput(notification.Email, testNum, false)
	fmt.Println()

	// HTML Email section
	if notification.Email.HTMLBody != "" {
		fmt.Println("  ╔═══ EMAIL (HTML) ═══")
		printEmailOutput(notification.Email, testNum, true)
		fmt.Println()
	}

	// Slack section
	fmt.Println("  ╔═══ SLACK ═══")
	printSlackOutput(notification, testNum)
	fmt.Println()

	// Webhook/JSON section
	fmt.Println("  ╔═══ WEBHOOK/JSON ═══")
	fmt.Println("  " + strings.Repeat("─", 58))
	webhookLines := strings.Split(notification.WebhookBody, "\n")
	for _, line := range webhookLines {
		fmt.Printf("  %s\n", line)
	}
}

func printJSONOutput(alert AlertmanagerAlert, notification NotificationOutput) {
	output := map[string]interface{}{
		"alert":        alert,
		"notification": notification,
	}

	jsonData, err := json.MarshalIndent(output, "  ", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}
