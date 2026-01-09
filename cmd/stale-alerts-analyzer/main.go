// Package main provides the stale-alerts-analyzer command for identifying and removing alerts that haven't fired recently.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/conallob/o11y-analysis-tools/internal/alertmanager"
)

func main() {
	var (
		prometheusURL = flag.String("prometheus-url", "http://localhost:9090", "Prometheus server URL")
		rulesFile     = flag.String("rules", "", "path to Prometheus rules file (required)")
		threshold     = flag.Duration("threshold", 8760*time.Hour, "time threshold for stale alerts (default: 8760h = 12 months)")
		fixMode       = flag.Bool("fix", false, "automatically delete stale alerts from rules file")
		verbose       = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: stale-alerts-analyzer [options]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze alerts to identify those that haven't fired recently.\n")
		fmt.Fprintf(os.Stderr, "Stale alerts may indicate outdated monitoring or resolved issues that no longer need alerting.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Check for stale alerts (default 12 months)\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --prometheus-url=http://prometheus:9090 --rules=./alerts.yml\n\n")
		fmt.Fprintf(os.Stderr, "  # Check with custom threshold (6 months)\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --rules=./alerts.yml --threshold=4380h\n\n")
		fmt.Fprintf(os.Stderr, "  # Fix mode: automatically delete stale alerts\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --fix --rules=./alerts.yml --threshold=8760h\n")
	}

	flag.Parse()

	if *prometheusURL == "" {
		fmt.Fprintf(os.Stderr, "Error: --prometheus-url is required\n")
		flag.Usage()
		os.Exit(1)
	}

	if *rulesFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --rules is required\n")
		flag.Usage()
		os.Exit(1)
	}

	if *threshold <= 0 {
		fmt.Fprintf(os.Stderr, "Error: --threshold must be positive\n")
		flag.Usage()
		os.Exit(1)
	}

	// Load alert names from rules file
	fmt.Printf("Loading alerts from %s...\n", *rulesFile)
	alertNames, err := alertmanager.GetAlertNamesFromRules(*rulesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading rules file: %v\n", err)
		os.Exit(1)
	}

	if len(alertNames) == 0 {
		fmt.Println("No alerts found in rules file")
		os.Exit(0)
	}

	fmt.Printf("Found %d alerts in rules file\n", len(alertNames))
	fmt.Println()

	// Query Prometheus for last firing times
	fmt.Printf("Querying Prometheus at %s...\n", *prometheusURL)
	fmt.Printf("Looking back %s for alert activity...\n", *threshold)
	fmt.Println()

	lastFired, err := alertmanager.FindLastFiredTimes(*prometheusURL, alertNames, *threshold, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}

	// Analyze results to find stale alerts
	now := time.Now()
	staleThreshold := now.Add(-*threshold)

	var staleAlerts []string
	var activeAlerts []string
	var neverFiredAlerts []string

	for _, alertName := range alertNames {
		lastTime := lastFired[alertName]

		switch {
		case lastTime.IsZero():
			// Never fired in lookback period
			neverFiredAlerts = append(neverFiredAlerts, alertName)
			staleAlerts = append(staleAlerts, alertName)
		case lastTime.Before(staleThreshold):
			// Fired, but before threshold
			staleAlerts = append(staleAlerts, alertName)
		default:
			// Recently active
			activeAlerts = append(activeAlerts, alertName)
		}
	}

	// Display results
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Analysis Results")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()

	if len(activeAlerts) > 0 {
		fmt.Printf("✓ Active Alerts (%d):\n", len(activeAlerts))
		fmt.Println("  These alerts have fired within the threshold period.")
		fmt.Println()
		for _, alertName := range activeAlerts {
			lastTime := lastFired[alertName]
			age := now.Sub(lastTime)
			fmt.Printf("  • %s\n", alertName)
			fmt.Printf("    Last fired: %s (%s ago)\n", lastTime.Format("2006-01-02 15:04:05"), formatDuration(age))
		}
		fmt.Println()
	}

	if len(staleAlerts) > 0 {
		fmt.Printf("⚠ Stale Alerts (%d):\n", len(staleAlerts))
		fmt.Printf("  These alerts have not fired in the last %s.\n", *threshold)
		fmt.Println()

		for _, alertName := range staleAlerts {
			lastTime := lastFired[alertName]
			fmt.Printf("  • %s\n", alertName)
			if lastTime.IsZero() {
				fmt.Printf("    Last fired: Never (within lookback period)\n")
			} else {
				age := now.Sub(lastTime)
				fmt.Printf("    Last fired: %s (%s ago)\n", lastTime.Format("2006-01-02 15:04:05"), formatDuration(age))
			}
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Summary")
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("Total alerts: %d\n", len(alertNames))
	fmt.Printf("Active alerts: %d\n", len(activeAlerts))
	fmt.Printf("Stale alerts: %d\n", len(staleAlerts))
	if len(neverFiredAlerts) > 0 {
		fmt.Printf("  - Never fired: %d\n", len(neverFiredAlerts))
		fmt.Printf("  - Fired but stale: %d\n", len(staleAlerts)-len(neverFiredAlerts))
	}
	fmt.Println()

	// Fix mode
	switch {
	case *fixMode:
		if len(staleAlerts) == 0 {
			fmt.Println("✓ No stale alerts to delete")
			os.Exit(0)
		}

		fmt.Printf("Fix mode: Deleting %d stale alerts from %s...\n", len(staleAlerts), *rulesFile)
		if err := alertmanager.DeleteAlertsFromRules(*rulesFile, staleAlerts); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting alerts: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✓ Successfully deleted stale alerts")
		fmt.Println()
		fmt.Println("Deleted alerts:")
		for _, alertName := range staleAlerts {
			fmt.Printf("  • %s\n", alertName)
		}
	case len(staleAlerts) > 0:
		fmt.Printf("Run with --fix to automatically delete these %d stale alerts\n", len(staleAlerts))
		os.Exit(1)
	default:
		fmt.Println("✓ No stale alerts found")
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 365 {
		months := days / 30
		return fmt.Sprintf("%d months", months)
	}
	years := days / 365
	return fmt.Sprintf("%d years", years)
}
