package formatting

import (
	"fmt"
	"regexp"
	"strings"
)

// CheckAndFormatPromQL analyzes YAML content for PromQL expressions and formats them
func CheckAndFormatPromQL(content string) ([]string, string) {
	var issues []string
	formatted := content

	// Find all PromQL expressions in YAML (expr: or query: fields)
	exprRegex := regexp.MustCompile(`(?m)^(\s*(?:expr|query):)\s*(.+)$`)

	matches := exprRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		fullMatch := match[0]
		prefix := match[1]
		expression := strings.TrimSpace(match[2])

		// Remove quotes if present
		if strings.HasPrefix(expression, `"`) && strings.HasSuffix(expression, `"`) {
			expression = strings.Trim(expression, `"`)
		} else if strings.HasPrefix(expression, "'") && strings.HasSuffix(expression, "'") {
			expression = strings.Trim(expression, "'")
		}

		// Check if expression should be multiline
		if shouldBeMultiline(expression) {
			issues = append(issues, fmt.Sprintf("Expression should use multiline formatting: %.60s...", expression))

			// Format the expression
			formattedExpr := formatPromQLMultiline(expression)

			// Replace in the content
			indentation := getIndentation(fullMatch)
			newBlock := formatYAMLBlock(prefix, formattedExpr, indentation)
			formatted = strings.Replace(formatted, fullMatch, newBlock, 1)
		}
	}

	return issues, formatted
}

// shouldBeMultiline determines if a PromQL expression should be formatted as multiline
func shouldBeMultiline(expr string) bool {
	// Expression should be multiline if:
	// 1. It's longer than 80 characters
	// 2. It contains binary operations with multiple clauses
	// 3. It has complex aggregations

	if len(expr) > 80 {
		return true
	}

	// Check for multiple operators suggesting complexity
	operatorCount := 0
	operators := []string{" and ", " or ", " unless ", " by ", " without ", " on ", " ignoring "}
	for _, op := range operators {
		operatorCount += strings.Count(strings.ToLower(expr), op)
	}

	return operatorCount >= 2
}

// formatPromQLMultiline formats a PromQL expression with proper multiline formatting
func formatPromQLMultiline(expr string) string {
	// Basic formatting rules:
	// 1. Put aggregation operators on separate lines
	// 2. Indent nested expressions
	// 3. Break long lines at logical operators

	lines := []string{}
	currentLine := ""
	depth := 0

	// Split by major operators while preserving them
	parts := splitByOperators(expr)

	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		// Detect opening/closing parentheses to manage depth
		openCount := strings.Count(trimmed, "(")
		closeCount := strings.Count(trimmed, ")")

		if i == 0 {
			currentLine = trimmed
		} else {
			// Check if this is an operator
			if isOperator(trimmed) {
				lines = append(lines, currentLine)
				currentLine = trimmed
			} else {
				if currentLine != "" && !isOperator(currentLine) {
					currentLine += " " + trimmed
				} else if isOperator(currentLine) {
					lines = append(lines, currentLine)
					currentLine = trimmed
				} else {
					currentLine = trimmed
				}
			}
		}

		depth += openCount - closeCount
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	// If we didn't split into multiple lines, return original
	if len(lines) <= 1 {
		return expr
	}

	return strings.Join(lines, "\n")
}

// splitByOperators splits a PromQL expression by major operators
func splitByOperators(expr string) []string {
	// Split by major operators while keeping them
	operators := []string{" and ", " or ", " unless ", " by(", " without(", " on(", " ignoring("}

	result := []string{expr}

	for _, op := range operators {
		newResult := []string{}
		for _, part := range result {
			if strings.Contains(strings.ToLower(part), op) {
				subparts := splitKeepDelimiter(part, op)
				newResult = append(newResult, subparts...)
			} else {
				newResult = append(newResult, part)
			}
		}
		result = newResult
	}

	return result
}

// splitKeepDelimiter splits string by delimiter but keeps the delimiter
func splitKeepDelimiter(s, delim string) []string {
	parts := strings.Split(strings.ToLower(s), delim)
	result := []string{}

	// Find actual positions in original string
	remaining := s
	for i, part := range parts {
		if i > 0 {
			// Add the delimiter
			result = append(result, strings.TrimSpace(delim))
		}

		if len(part) > 0 {
			// Find this part in remaining string (case-insensitive search)
			idx := strings.Index(strings.ToLower(remaining), part)
			if idx >= 0 {
				actual := remaining[idx : idx+len(part)]
				result = append(result, actual)
				remaining = remaining[idx+len(part):]
			}
		}
	}

	return result
}

// isOperator checks if a string is an operator
func isOperator(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	operators := []string{"and", "or", "unless", "by(", "without(", "on(", "ignoring("}

	for _, op := range operators {
		if strings.HasPrefix(s, op) || s == strings.TrimSuffix(op, "(") {
			return true
		}
	}

	return false
}

// getIndentation extracts the indentation from a line
func getIndentation(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return ""
}

// formatYAMLBlock formats a YAML block with multiline string
func formatYAMLBlock(prefix, expr, indentation string) string {
	if !strings.Contains(expr, "\n") {
		return fmt.Sprintf("%s %s", prefix, expr)
	}

	lines := strings.Split(expr, "\n")
	result := prefix + " |\n"

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result += indentation + "  " + strings.TrimSpace(line) + "\n"
		}
	}

	return strings.TrimRight(result, "\n")
}
