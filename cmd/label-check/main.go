// Package main provides the label-check command for validating required labels in PromQL expressions.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/conallob/o11y-analysis-tools/internal/promql"
)

func main() {
	var (
		requiredLabels      = flag.String("labels", "job", "comma-separated list of required labels (default: job)")
		requiredAlertLabels = flag.String("alert-labels", "", "comma-separated list of required alert annotation labels (e.g., severity,grafana_url,runbook)")
		checkAlerts         = flag.Bool("check-alerts", false, "enable alert-specific label validation")
	)

	// Define flags for future functionality
	_ = flag.Bool("verbose", false, "verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: label-check [options] <file|directory>...\n\n")
		fmt.Fprintf(os.Stderr, "Enforce label standards in PromQL expressions and alerts.\n")
		fmt.Fprintf(os.Stderr, "Ensures required labels are present to prevent collisions in multi-tenant platforms.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  label-check --labels=job,namespace ./alerts\n")
		fmt.Fprintf(os.Stderr, "  label-check --check-alerts --alert-labels=severity,grafana_url,runbook ./alerts\n")
		fmt.Fprintf(os.Stderr, "  echo 'rate(metric[5m])' | label-check --labels=job -\n")
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// Parse required labels
	labels := strings.Split(*requiredLabels, ",")
	for i := range labels {
		labels[i] = strings.TrimSpace(labels[i])
	}

	var alertLabels []string
	if *checkAlerts && *requiredAlertLabels != "" {
		alertLabels = strings.Split(*requiredAlertLabels, ",")
		for i := range alertLabels {
			alertLabels[i] = strings.TrimSpace(alertLabels[i])
		}
	}

	exitCode := 0
	totalExpressions := 0
	violationCount := 0
	totalAlerts := 0
	alertViolationCount := 0

	for _, path := range flag.Args() {
		// Handle stdin input
		if path == "-" {
			content, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				exitCode = 1
				continue
			}

			violations := promql.CheckRequiredLabels(string(content), labels)
			totalExpressions += len(violations)

			for _, v := range violations {
				if len(v.MissingLabels) > 0 {
					violationCount++
					fmt.Printf("Expression: %s\n", truncate(v.Expression, 60))
					fmt.Printf("  Missing required labels: %s\n", strings.Join(v.MissingLabels, ", "))
					exitCode = 1
				}
			}
			continue
		}

		err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			// Process YAML files
			if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
				return nil
			}

			content, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", filePath, err)
				exitCode = 1
				return nil
			}

			violations := promql.CheckRequiredLabels(string(content), labels)
			totalExpressions += len(violations)

			hasViolation := false
			for _, v := range violations {
				if len(v.MissingLabels) > 0 {
					if !hasViolation {
						fmt.Printf("%s:\n", filePath)
						hasViolation = true
						exitCode = 1
					}
					violationCount++
					fmt.Printf("  Expression: %s\n", truncate(v.Expression, 60))
					fmt.Printf("    Missing required labels: %s\n", strings.Join(v.MissingLabels, ", "))
					if v.Line > 0 {
						fmt.Printf("    Line: %d\n", v.Line)
					}
				}
			}

			if hasViolation {
				fmt.Println()
			}

			// Check alert-specific labels if enabled
			if *checkAlerts && len(alertLabels) > 0 {
				alertViolations := promql.CheckAlertLabels(string(content), alertLabels)
				totalAlerts += len(alertViolations)

				hasAlertViolation := false
				for _, v := range alertViolations {
					if len(v.MissingLabels) > 0 {
						if !hasAlertViolation {
							if !hasViolation {
								fmt.Printf("%s:\n", filePath)
							}
							hasAlertViolation = true
							exitCode = 1
						}
						alertViolationCount++
						fmt.Printf("  Alert: %s\n", v.AlertName)
						fmt.Printf("    Missing required alert labels: %s\n", strings.Join(v.MissingLabels, ", "))
						if v.Line > 0 {
							fmt.Printf("    Line: %d\n", v.Line)
						}
					}
				}

				if hasAlertViolation {
					fmt.Println()
				}
			}

			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			exitCode = 1
		}
	}

	if violationCount > 0 {
		fmt.Printf("Found %d expressions with missing required labels\n", violationCount)
		fmt.Printf("Required labels: %s\n", strings.Join(labels, ", "))
	} else if totalExpressions > 0 {
		fmt.Printf("All %d expressions have required labels\n", totalExpressions)
	}

	if *checkAlerts && alertViolationCount > 0 {
		fmt.Printf("Found %d alerts with missing required labels\n", alertViolationCount)
		fmt.Printf("Required alert labels: %s\n", strings.Join(alertLabels, ", "))
	} else if *checkAlerts && totalAlerts > 0 {
		fmt.Printf("All %d alerts have required labels\n", totalAlerts)
	}

	os.Exit(exitCode)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
