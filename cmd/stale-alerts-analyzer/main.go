// Package main provides the stale-alerts-analyzer command for identifying and removing alerts that haven't fired recently.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/conallob/o11y-analysis-tools/internal/alertmanager"
)

// parseDuration parses a duration string supporting extended units:
// - Standard Go units: ns, us/µs, ms, s, m, h
// - Extended units: d (days), w (weeks), M (months ~30 days), y (years ~365 days)
func parseDuration(s string) (time.Duration, error) {
	// Try standard Go duration parsing first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Parse extended duration formats
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)(y|M|w|d)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration format: %s (supported units: h, m, s, d, w, M, y)", s)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value: %s", matches[1])
	}

	unit := matches[2]
	var duration time.Duration

	switch unit {
	case "d":
		duration = time.Duration(value * 24 * float64(time.Hour))
	case "w":
		duration = time.Duration(value * 7 * 24 * float64(time.Hour))
	case "M":
		duration = time.Duration(value * 30 * 24 * float64(time.Hour))
	case "y":
		duration = time.Duration(value * 365 * 24 * float64(time.Hour))
	default:
		return 0, fmt.Errorf("unsupported duration unit: %s", unit)
	}

	return duration, nil
}

// formatDurationHuman formats a duration in human-readable format
func formatDurationHuman(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// Convert to largest sensible unit
	hours := d.Hours()

	switch {
	case hours >= 8760: // ~1 year
		years := hours / 8760
		if years == float64(int(years)) {
			return fmt.Sprintf("%dy", int(years))
		}
		return fmt.Sprintf("%.1fy", years)
	case hours >= 720: // ~1 month
		months := hours / 720
		if months == float64(int(months)) {
			return fmt.Sprintf("%dM", int(months))
		}
		return fmt.Sprintf("%.1fM", months)
	case hours >= 168: // 1 week
		weeks := hours / 168
		if weeks == float64(int(weeks)) {
			return fmt.Sprintf("%dw", int(weeks))
		}
		return fmt.Sprintf("%.1fw", weeks)
	case hours >= 24: // 1 day
		days := hours / 24
		if days == float64(int(days)) {
			return fmt.Sprintf("%dd", int(days))
		}
		return fmt.Sprintf("%.1fd", days)
	default:
		return d.String()
	}
}

func main() {
	var (
		prometheusURL  = flag.String("prometheus-url", "http://localhost:9090", "Prometheus server URL")
		rulesFile      = flag.String("rules", "", "path to Prometheus rules file (required)")
		timeHorizonStr = flag.String("timehorizon", "12M", "time horizon for stale alerts (units: h=hours, d=days, w=weeks, M=months, y=years)")
		fixMode        = flag.Bool("fix", false, "automatically delete stale alerts from rules file")
		verbose        = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: stale-alerts-analyzer [options]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze alerts to identify those that haven't fired recently.\n")
		fmt.Fprintf(os.Stderr, "Stale alerts may indicate outdated monitoring or resolved issues that no longer need alerting.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nTime Horizon Units:\n")
		fmt.Fprintf(os.Stderr, "  h  - hours\n")
		fmt.Fprintf(os.Stderr, "  d  - days\n")
		fmt.Fprintf(os.Stderr, "  w  - weeks\n")
		fmt.Fprintf(os.Stderr, "  M  - months (30 days)\n")
		fmt.Fprintf(os.Stderr, "  y  - years (365 days)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Check for stale alerts (default 12 months)\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --prometheus-url=http://prometheus:9090 --rules=./alerts.yml\n\n")
		fmt.Fprintf(os.Stderr, "  # Check with custom time horizon (6 months)\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --rules=./alerts.yml --timehorizon=6M\n\n")
		fmt.Fprintf(os.Stderr, "  # Check with time horizon in days (90 days)\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --rules=./alerts.yml --timehorizon=90d\n\n")
		fmt.Fprintf(os.Stderr, "  # Fix mode: automatically delete stale alerts\n")
		fmt.Fprintf(os.Stderr, "  stale-alerts-analyzer --fix --rules=./alerts.yml --timehorizon=1y\n")
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

	// Parse time horizon
	timeHorizon, err := parseDuration(*timeHorizonStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid --timehorizon value: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if timeHorizon <= 0 {
		fmt.Fprintf(os.Stderr, "Error: --timehorizon must be positive\n")
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
	fmt.Printf("Looking back %s for alert activity...\n", formatDurationHuman(timeHorizon))
	fmt.Println()

	lastFired, err := alertmanager.FindLastFiredTimes(*prometheusURL, alertNames, timeHorizon, *verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error querying Prometheus: %v\n", err)
		os.Exit(1)
	}

	// Analyze results to find stale alerts
	now := time.Now()
	staleThreshold := now.Add(-timeHorizon)

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
		fmt.Printf("  These alerts have fired within the time horizon (%s).\n", formatDurationHuman(timeHorizon))
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
		fmt.Printf("  These alerts have not fired in the last %s.\n", formatDurationHuman(timeHorizon))
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
