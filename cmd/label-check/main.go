package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/conallob/o11y-analysis-tools/internal/promql"
)

func main() {
	var (
		requiredLabels = flag.String("labels", "job", "comma-separated list of required labels (default: job)")
	)

	// Define flags for future functionality
	_ = flag.Bool("verbose", false, "verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: label-check [options] <file|directory>...\n\n")
		fmt.Fprintf(os.Stderr, "Enforce label standards in PromQL expressions.\n")
		fmt.Fprintf(os.Stderr, "Ensures required labels are present to prevent collisions in multi-tenant platforms.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  label-check --labels=job,namespace ./alerts\n")
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

	exitCode := 0
	totalExpressions := 0
	violationCount := 0

	for _, path := range flag.Args() {
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

			content, err := ioutil.ReadFile(filePath)
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

	os.Exit(exitCode)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
