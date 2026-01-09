// Package promql provides utilities for parsing and analyzing PromQL expressions.
package promql

import (
	"fmt"
	"regexp"
	"strings"
)

// LabelViolation represents a PromQL expression that's missing required labels
type LabelViolation struct {
	Expression    string
	MissingLabels []string
	Line          int
	Suggestion    string
}

// AlertViolation represents an alert that's missing required labels
type AlertViolation struct {
	AlertName     string
	MissingLabels []string
	Line          int
}

// CheckRequiredLabels checks PromQL expressions for required labels
func CheckRequiredLabels(content string, requiredLabels []string) []LabelViolation {
	var violations []LabelViolation

	// Find all PromQL expressions in YAML
	exprRegex := regexp.MustCompile(`(?m)^(\s*(?:expr|query):)\s*(.+?)$`)

	lines := strings.Split(content, "\n")
	for lineNum, line := range lines {
		matches := exprRegex.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}

		expression := strings.TrimSpace(matches[2])

		// Remove quotes
		expression = strings.Trim(expression, `"'`)

		// Check for required labels
		missingLabels := checkLabelsInExpression(expression, requiredLabels)

		violation := LabelViolation{
			Expression:    expression,
			MissingLabels: missingLabels,
			Line:          lineNum + 1,
		}

		if len(missingLabels) > 0 {
			violation.Suggestion = generateSuggestion(expression, missingLabels)
		}

		violations = append(violations, violation)
	}

	return violations
}

// checkLabelsInExpression checks if an expression contains all required labels
func checkLabelsInExpression(expr string, requiredLabels []string) []string {
	var missing []string

	// Parse the expression to find label matchers
	presentLabels := extractLabelsFromExpression(expr)

	for _, required := range requiredLabels {
		found := false
		for _, present := range presentLabels {
			if present == required {
				found = true
				break
			}
		}

		if !found {
			missing = append(missing, required)
		}
	}

	return missing
}

// extractLabelsFromExpression extracts all label names from a PromQL expression
func extractLabelsFromExpression(expr string) []string {
	labels := make(map[string]bool)

	// Match label matchers: label="value", label=~"regex", label!="value", label!~"regex"
	labelRegex := regexp.MustCompile(`(\w+)\s*(!?=~?)\s*"[^"]*"`)

	matches := labelRegex.FindAllStringSubmatch(expr, -1)
	for _, match := range matches {
		if len(match) > 1 {
			labelName := match[1]
			// Filter out PromQL keywords
			if !isPromQLKeyword(labelName) {
				labels[labelName] = true
			}
		}
	}

	// Also check for 'by' and 'without' clauses
	byWithoutRegex := regexp.MustCompile(`(?:by|without)\s*\(([^)]+)\)`)
	matches = byWithoutRegex.FindAllStringSubmatch(expr, -1)
	for _, match := range matches {
		if len(match) > 1 {
			labelList := strings.Split(match[1], ",")
			for _, label := range labelList {
				label = strings.TrimSpace(label)
				if label != "" && !isPromQLKeyword(label) {
					labels[label] = true
				}
			}
		}
	}

	result := make([]string, 0, len(labels))
	for label := range labels {
		result = append(result, label)
	}

	return result
}

// isPromQLKeyword checks if a string is a PromQL keyword
func isPromQLKeyword(s string) bool {
	keywords := map[string]bool{
		"and":         true,
		"or":          true,
		"unless":      true,
		"by":          true,
		"without":     true,
		"on":          true,
		"ignoring":    true,
		"group_left":  true,
		"group_right": true,
		"bool":        true,
		"offset":      true,
	}

	return keywords[strings.ToLower(s)]
}

// generateSuggestion generates a suggestion for adding missing labels
func generateSuggestion(expr string, missingLabels []string) string {
	// Find the metric name (first word before '{' or '(')
	metricRegex := regexp.MustCompile(`^(\w+)`)
	match := metricRegex.FindString(expr)

	if match == "" {
		return "Add required labels to the query selector"
	}

	// Check if there's already a label selector
	if strings.Contains(expr, "{") {
		// Suggest adding to existing selector
		labels := make([]string, len(missingLabels))
		for i, label := range missingLabels {
			labels[i] = label + "=\"...\""
		}
		labelStr := strings.Join(labels, ", ")
		return fmt.Sprintf("Add %s to the label matcher", labelStr)
	}

	// No label selector exists, suggest adding one
	labels := make([]string, len(missingLabels))
	for i, label := range missingLabels {
		labels[i] = label + "=\"...\""
	}

	return "Add label matcher: " + match + "{" + strings.Join(labels, ", ") + "}"
}

// CheckAlertLabels checks that alerts have required labels in their labels section
func CheckAlertLabels(content string, requiredLabels []string) []AlertViolation {
	var violations []AlertViolation

	// Parse YAML to find alert definitions
	lines := strings.Split(content, "\n")

	var currentAlert string
	var currentAlertLine int
	var alertLabels []string
	inLabelsSection := false
	labelsIndent := 0

	for lineNum, line := range lines {
		// Check for alert definition
		alertMatch := regexp.MustCompile(`^\s*-\s*alert:\s*(\S+)`).FindStringSubmatch(line)
		if len(alertMatch) > 1 {
			// If we were processing a previous alert, check it
			if currentAlert != "" {
				missing := checkAlertLabels(alertLabels, requiredLabels)
				if len(missing) > 0 {
					violations = append(violations, AlertViolation{
						AlertName:     currentAlert,
						MissingLabels: missing,
						Line:          currentAlertLine,
					})
				}
			}

			// Start new alert
			currentAlert = alertMatch[1]
			currentAlertLine = lineNum + 1
			alertLabels = nil
			inLabelsSection = false
			continue
		}

		// Check for labels section
		if currentAlert != "" {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))

			// Check if we're entering the labels section
			if regexp.MustCompile(`^\s*labels:\s*$`).MatchString(line) {
				inLabelsSection = true
				labelsIndent = indent
				continue
			}

			// If we're in the labels section, collect label names
			if inLabelsSection {
				// Check if we've left the labels section (indent decreased or new section started)
				if indent <= labelsIndent || regexp.MustCompile(`^\s*\w+:\s*`).MatchString(line) && indent == labelsIndent {
					inLabelsSection = false
				} else {
					// Extract label name
					labelMatch := regexp.MustCompile(`^\s*(\w+):\s*`).FindStringSubmatch(line)
					if len(labelMatch) > 1 {
						alertLabels = append(alertLabels, labelMatch[1])
					}
				}
			}
		}
	}

	// Check the last alert if any
	if currentAlert != "" {
		missing := checkAlertLabels(alertLabels, requiredLabels)
		if len(missing) > 0 {
			violations = append(violations, AlertViolation{
				AlertName:     currentAlert,
				MissingLabels: missing,
				Line:          currentAlertLine,
			})
		}
	}

	return violations
}

// checkAlertLabels checks if an alert has all required labels
func checkAlertLabels(alertLabels []string, requiredLabels []string) []string {
	var missing []string

	for _, required := range requiredLabels {
		found := false
		for _, present := range alertLabels {
			if present == required {
				found = true
				break
			}
		}

		if !found {
			missing = append(missing, required)
		}
	}

	return missing
}
