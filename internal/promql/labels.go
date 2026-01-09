package promql

import (
	"fmt"
	"regexp"
	"strings"
)

// LabelViolation represents a PromQL expression that's missing required labels
type LabelViolation struct {
	Expression     string
	MissingLabels  []string
	Line           int
	Suggestion     string
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
		"and":      true,
		"or":       true,
		"unless":   true,
		"by":       true,
		"without":  true,
		"on":       true,
		"ignoring": true,
		"group_left": true,
		"group_right": true,
		"bool":     true,
		"offset":   true,
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
