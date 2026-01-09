// Package main provides the alert-hysteresis command for analyzing alert firing patterns.
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
		prometheusURL    = flag.String("prometheus-url", "http://localhost:9090", "Prometheus server URL")
		alertName        = flag.String("alert", "", "specific alert name to analyze (optional)")
		timeframe        = flag.Duration("timeframe", 7*24*time.Hour, "timeframe to analyze (default: 7 days)")
		threshold        = flag.Float64("threshold", 0.2, "threshold for suggesting changes (20% mismatch)")
		rulesFile        = flag.String("rules", "", "path to Prometheus rules file to compare against")
		fixMode          = flag.Bool("fix", false, "automatically update rules file with recommendations (requires --rules and --target-percentile)")
		targetPercentile = flag.Float64("target-percentile", 0.3, "target percentile for alert threshold (0-1, default: 0.3)")
		verbose          = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: alert-hysteresis [options]\n\n")
		fmt.Fprintf(os.Stderr, "Analyze alert firing patterns and suggest optimal 'for' durations.\n")
		fmt.Fprintf(os.Stderr, "Reduces spurious, unactionable alerts by comparing actual firing durations\n")
		fmt.Fprintf(os.Stderr, "with configured hysteresis values.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  # Analyze alerts (check mode)\n")
		fmt.Fprintf(os.Stderr, "  alert-hysteresis --prometheus-url=http://prometheus:9090 --timeframe=24h\n")
		fmt.Fprintf(os.Stderr, "  alert-hysteresis --alert=HighErrorRate --rules=./alerts.yml\n\n")
		fmt.Fprintf(os.Stderr, "  # Fix mode: update rules file with recommendations\n")
		fmt.Fprintf(os.Stderr, "  alert-hysteresis --fix --rules=./alerts.yml --target-percentile=0.25\n")
		fmt.Fprintf(os.Stderr, "  alert-hysteresis --fix --rules=./alerts.yml --target-percentile=0.5\n")
	}

	flag.Parse()

	if *prometheusURL == "" {
		fmt.Fprintf(os.Stderr, "Error: --prometheus-url is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate fix mode requirements
	if *fixMode {
		if *rulesFile == "" {
			fmt.Fprintf(os.Stderr, "Error: --fix mode requires --rules to be specified\n")
			flag.Usage()
			os.Exit(1)
		}
		if *targetPercentile < 0 || *targetPercentile > 1 {
			fmt.Fprintf(os.Stderr, "Error: --target-percentile must be between 0 and 1\n")
			flag.Usage()
			os.Exit(1)
		}
	}

	// Create analyzer
	analyzer := alertmanager.NewHysteresisAnalyzer(*prometheusURL, *verbose)

	// Fetch alert history
	fmt.Printf("Fetching alert history from %s (timeframe: %s)...\n", *prometheusURL, *timeframe)

	history, err := analyzer.FetchAlertHistory(*timeframe, *alertName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching alert history: %v\n", err)
		os.Exit(1)
	}

	if len(history) == 0 {
		fmt.Println("No alert history found in the specified timeframe")
		os.Exit(0)
	}

	fmt.Printf("Analyzing %d alert firing events...\n", len(history))
	if *fixMode {
		fmt.Printf("Fix mode: using target percentile %.0f%%\n", *targetPercentile*100)
	}
	fmt.Println()

	// Load configured 'for' durations from rules file if provided
	var configuredDurations map[string]time.Duration
	if *rulesFile != "" {
		configuredDurations, err = alertmanager.LoadAlertDurations(*rulesFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not load rules file: %v\n", err)
		}
	}

	// Analyze each alert
	exitCode := 0
	recommendations := 0
	recommendedUpdates := make(map[string]time.Duration)
	totalPreventedAlerts := 0

	for alertName, events := range history {
		// Use target percentile for analysis
		analysis := analyzer.AnalyzeAlertWithPercentile(alertName, events, *targetPercentile)

		// Get configured duration
		var configuredFor time.Duration
		if configuredDurations != nil {
			if duration, ok := configuredDurations[alertName]; ok {
				configuredFor = duration
			}
		}

		// Check if recommendation is needed
		needsAdjustment := false
		if configuredFor > 0 {
			mismatch := calculateMismatch(analysis.RecommendedFor, configuredFor)
			if mismatch > *threshold {
				needsAdjustment = true
				exitCode = 1
				recommendations++
				recommendedUpdates[alertName] = analysis.RecommendedFor
			}
		} else if *fixMode {
			// In fix mode, recommend for all alerts even without current config
			recommendedUpdates[alertName] = analysis.RecommendedFor
		}

		// Track total prevented alerts
		totalPreventedAlerts += analysis.PreventedAlerts

		// Print analysis
		fmt.Printf("Alert: %s\n", alertName)
		fmt.Printf("  Firing events: %d\n", analysis.FiringCount)
		fmt.Printf("  Average duration: %s\n", analysis.AvgDuration.Round(time.Second))
		fmt.Printf("  Median duration (P50): %s\n", analysis.MedianDuration.Round(time.Second))
		fmt.Printf("  75th percentile (P75): %s\n", analysis.P75Duration.Round(time.Second))
		fmt.Printf("  90th percentile (P90): %s\n", analysis.P90Duration.Round(time.Second))
		fmt.Printf("  Min/Max duration: %s / %s\n",
			analysis.MinDuration.Round(time.Second),
			analysis.MaxDuration.Round(time.Second))

		if configuredFor > 0 {
			fmt.Printf("  Configured 'for': %s\n", configuredFor.Round(time.Second))
		}

		if needsAdjustment {
			fmt.Printf("  ⚠ RECOMMENDATION: Change 'for' duration to %s\n",
				analysis.RecommendedFor.Round(time.Second))
			fmt.Printf("     Reason: %s\n", analysis.Reasoning)
			if analysis.PreventedAlerts > 0 {
				fmt.Printf("     Impact: Would have prevented %d/%d alerts (%.1f%%)\n",
					analysis.PreventedAlerts,
					analysis.FiringCount,
					float64(analysis.PreventedAlerts)/float64(analysis.FiringCount)*100)
			}
		} else if analysis.RecommendedFor > 0 {
			fmt.Printf("  Recommended 'for': %s (based on P%.0f)\n",
				analysis.RecommendedFor.Round(time.Second),
				analysis.TargetPercentile*100)
			if configuredFor > 0 {
				fmt.Printf("  ✓ Current configuration is acceptable\n")
			}
			if analysis.PreventedAlerts > 0 {
				fmt.Printf("  Impact: Would prevent %d/%d alerts (%.1f%%)\n",
					analysis.PreventedAlerts,
					analysis.FiringCount,
					float64(analysis.PreventedAlerts)/float64(analysis.FiringCount)*100)
			}
		}

		// Show spurious alerts count
		if analysis.SpuriousAlerts > 0 {
			fmt.Printf("  Spurious alerts (< recommended): %d (%.1f%%)\n",
				analysis.SpuriousAlerts,
				float64(analysis.SpuriousAlerts)/float64(analysis.FiringCount)*100)
		}

		fmt.Println()
	}

	// Summary
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("Summary")
	fmt.Println("═══════════════════════════════════════════════════════════")

	switch {
	case len(recommendedUpdates) > 0:
		fmt.Printf("Found %d alerts with recommendations\n", len(recommendedUpdates))
		fmt.Printf("Total alerts that would be prevented: %d\n", totalPreventedAlerts)
		fmt.Println()

		if *fixMode {
			// Apply fixes to rules file
			fmt.Printf("Applying fixes to %s...\n", *rulesFile)
			if err := alertmanager.UpdateAlertDurations(*rulesFile, recommendedUpdates); err != nil {
				fmt.Fprintf(os.Stderr, "Error updating rules file: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✓ Rules file updated successfully")
			fmt.Println()
			fmt.Println("Updated alerts:")
			for alertName, newDuration := range recommendedUpdates {
				oldDuration := configuredDurations[alertName]
				fmt.Printf("  %s: %s → %s\n",
					alertName,
					formatDuration(oldDuration),
					formatDuration(newDuration))
			}
		} else {
			fmt.Println("Recommended updates:")
			for alertName, newDuration := range recommendedUpdates {
				oldDuration := configuredDurations[alertName]
				fmt.Printf("  %s: %s → %s\n",
					alertName,
					formatDuration(oldDuration),
					formatDuration(newDuration))
			}
			fmt.Println()
			fmt.Printf("Run with --fix to automatically apply these changes\n")
			fmt.Printf("Adjust --target-percentile (current: %.0f%%) to change sensitivity\n",
				*targetPercentile*100)
		}
	case recommendations > 0:
		fmt.Printf("Found %d alerts that need hysteresis adjustment\n", recommendations)
		fmt.Printf("Run with --rules=<path> to compare against configured values\n")
	default:
		fmt.Printf("✓ All alerts have appropriate hysteresis values\n")
	}

	os.Exit(exitCode)
}

// calculateMismatch calculates the percentage mismatch between two durations
func calculateMismatch(recommended, configured time.Duration) float64 {
	if configured == 0 {
		return 0
	}

	diff := float64(recommended - configured)
	if diff < 0 {
		diff = -diff
	}

	return diff / float64(configured)
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "(none)"
	}
	return d.Round(time.Second).String()
}
