package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/conallob/o11y-analysis-tools/pkg/formatting"
)

func main() {
	var (
		fix     = flag.Bool("fix", false, "automatically fix formatting issues")
		fmt_    = flag.Bool("fmt", false, "automatically fix formatting issues (alias for --fix)")
		check   = flag.Bool("check", true, "check formatting without fixing (default)")
		verbose = flag.Bool("verbose", false, "verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: promql-fmt [options] <file|directory>...\n\n")
		fmt.Fprintf(os.Stderr, "Static analysis tool for PromQL expression formatting.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// --fix and --fmt are aliases
	shouldFix := *fix || *fmt_
	shouldCheck := *check && !shouldFix

	exitCode := 0
	totalFiles := 0
	filesWithIssues := 0

	for _, path := range flag.Args() {
		err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Skip directories
			if info.IsDir() {
				return nil
			}

			// Process YAML files (Prometheus rules/alerts)
			if !strings.HasSuffix(filePath, ".yaml") && !strings.HasSuffix(filePath, ".yml") {
				return nil
			}

			totalFiles++

			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", filePath, err)
				exitCode = 1
				return nil
			}

			issues, formatted := formatting.CheckAndFormatPromQL(string(content))

			if len(issues) > 0 {
				filesWithIssues++
				if shouldCheck {
					fmt.Printf("%s:\n", filePath)
					for _, issue := range issues {
						fmt.Printf("  - %s\n", issue)
					}
					exitCode = 1
				}
			}

			if shouldFix && formatted != string(content) {
				if *verbose {
					fmt.Printf("Fixing %s\n", filePath)
				}
				if err := ioutil.WriteFile(filePath, []byte(formatted), info.Mode()); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", filePath, err)
					exitCode = 1
				}
			}

			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			exitCode = 1
		}
	}

	if shouldCheck {
		if filesWithIssues > 0 {
			fmt.Printf("\nFound formatting issues in %d/%d files\n", filesWithIssues, totalFiles)
			fmt.Printf("Run with --fix to automatically format\n")
		} else if totalFiles > 0 {
			fmt.Printf("All %d files are properly formatted\n", totalFiles)
		}
	} else if shouldFix {
		if *verbose || filesWithIssues > 0 {
			fmt.Printf("Formatted %d/%d files\n", filesWithIssues, totalFiles)
		}
	}

	os.Exit(exitCode)
}
