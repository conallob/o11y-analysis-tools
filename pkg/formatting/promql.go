// Package formatting provides utilities for formatting and validating PromQL expressions.
package formatting

import (
	"fmt"
	"regexp"
	"strings"
)

// CheckOptions configures the behavior of CheckAndFormatPromQL
type CheckOptions struct {
	DisableLineLength bool
}

// AggregationStyle tracks the position of aggregation clauses
type AggregationStyle int

// Aggregation clause positioning styles
const (
	// AggregationStyleUnknown indicates no aggregation clause detected
	AggregationStyleUnknown AggregationStyle = iota
	// AggregationStylePostfix indicates postfix style: sum(metric) by (label)
	AggregationStylePostfix
	// AggregationStylePrefix indicates prefix style: sum by (label) (metric)
	AggregationStylePrefix
)

// CheckAndFormatPromQL analyzes YAML content for PromQL expressions and formats them
func CheckAndFormatPromQL(content string, opts CheckOptions) ([]string, string) {
	var issues []string
	formatted := content

	// Track aggregation clause positioning for consistency
	var dominantStyle AggregationStyle
	styleCount := make(map[AggregationStyle]int)

	// Find all PromQL expressions in YAML (expr: or query: fields)
	exprRegex := regexp.MustCompile(`(?m)^(\s*(?:expr|query):)\s*(.+)$`)

	matches := exprRegex.FindAllStringSubmatch(content, -1)

	// First pass: detect dominant style
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		expression := strings.TrimSpace(match[2])
		if strings.HasPrefix(expression, `"`) && strings.HasSuffix(expression, `"`) {
			expression = strings.Trim(expression, `"`)
		} else if strings.HasPrefix(expression, "'") && strings.HasSuffix(expression, "'") {
			expression = strings.Trim(expression, "'")
		}

		style := detectAggregationStyle(expression)
		if style != AggregationStyleUnknown {
			styleCount[style]++
		}
	}

	// Determine dominant style
	postfixCount := styleCount[AggregationStylePostfix]
	prefixCount := styleCount[AggregationStylePrefix]

	switch {
	case postfixCount > prefixCount:
		dominantStyle = AggregationStylePostfix
	case prefixCount > postfixCount:
		dominantStyle = AggregationStylePrefix
	case postfixCount > 0:
		// If equal, prefer postfix as it's more common
		dominantStyle = AggregationStylePostfix
	}

	// Second pass: check each expression
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
		if shouldBeMultiline(expression, opts.DisableLineLength) {
			issues = append(issues, fmt.Sprintf("Expression should use multiline formatting: %.60s...", expression))

			// Format the expression
			formattedExpr := formatPromQLMultiline(expression)

			// Replace in the content
			indentation := getIndentation(fullMatch)
			newBlock := formatYAMLBlock(prefix, formattedExpr, indentation)
			formatted = strings.Replace(formatted, fullMatch, newBlock, 1)
		}

		// Check Prometheus best practices
		bestPracticeIssues := checkPrometheusBestPractices(expression)
		issues = append(issues, bestPracticeIssues...)

		// Check aggregation clause consistency
		if dominantStyle != AggregationStyleUnknown {
			style := detectAggregationStyle(expression)
			if style != AggregationStyleUnknown && style != dominantStyle {
				styleName := map[AggregationStyle]string{
					AggregationStylePostfix: "postfix (e.g., 'sum(metric) by (label)')",
					AggregationStylePrefix:  "prefix (e.g., 'sum by (label) (metric)')",
				}
				issues = append(issues, fmt.Sprintf("Inconsistent aggregation clause positioning: expression uses %s style, but file predominantly uses %s",
					styleName[style], styleName[dominantStyle]))
			}
		}
	}

	return issues, formatted
}

// detectAggregationStyle determines the positioning style of aggregation clauses in an expression
func detectAggregationStyle(expr string) AggregationStyle {
	// Aggregation operators that can have by/without clauses
	aggregationOps := []string{"sum", "min", "max", "avg", "group", "stddev", "stdvar", "count", "count_values",
		"bottomk", "topk", "quantile"}

	// Look for patterns like: sum by (labels) (expr) or sum without (labels) (expr)
	// Prefix style: aggregation by/without (...) (...)
	// This must be checked first because it's more specific
	for _, op := range aggregationOps {
		prefixPattern := fmt.Sprintf(`\b%s\s+(by|without)\s*\([^)]*\)\s*\(`, op)
		prefixRegex := regexp.MustCompile(prefixPattern)
		if prefixRegex.MatchString(expr) {
			return AggregationStylePrefix
		}
	}

	// Look for patterns like: sum(...) by (labels) or sum(...) without (labels)
	// Postfix style: aggregation(...) by/without (...)
	// Use a simpler approach: find aggregation op followed eventually by ) then by/without
	postfixPattern := `\b(sum|min|max|avg|group|stddev|stdvar|count|count_values|bottomk|topk|quantile)\s*\(.*\)\s*(by|without)\s*\(`
	postfixRegex := regexp.MustCompile(postfixPattern)
	if postfixRegex.MatchString(expr) {
		return AggregationStylePostfix
	}

	return AggregationStyleUnknown
}

// shouldBeMultiline determines if a PromQL expression should be formatted as multiline
func shouldBeMultiline(expr string, disableLineLength bool) bool {
	// Expression should be multiline if:
	// 1. It's longer than 80 characters (unless disabled)
	// 2. It contains binary operations with multiple clauses
	// 3. It has complex aggregations

	if !disableLineLength && len(expr) > 80 {
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
				switch {
				case currentLine != "" && !isOperator(currentLine):
					currentLine += " " + trimmed
				case isOperator(currentLine):
					lines = append(lines, currentLine)
					currentLine = trimmed
				default:
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

// checkPrometheusBestPractices validates PromQL expressions against Prometheus best practices
func checkPrometheusBestPractices(expr string) []string {
	var issues []string

	// Extract metric names from the expression
	metricNames := extractMetricNames(expr)

	for _, metricName := range metricNames {
		// Check naming conventions
		issues = append(issues, checkMetricNamingConventions(metricName)...)

		// Check for proper suffixes
		issues = append(issues, checkMetricSuffixes(metricName)...)
	}

	// Check for instrumentation best practices
	issues = append(issues, checkInstrumentationPatterns(expr)...)

	return issues
}

// extractMetricNames extracts metric names from a PromQL expression
func extractMetricNames(expr string) []string {
	// Match metric names: alphanumeric with underscores, before { or [ or space or ) or end of string
	metricRegex := regexp.MustCompile(`\b([a-zA-Z_:][a-zA-Z0-9_:]*)\s*(?:[{\[\s)]|$)`)
	matches := metricRegex.FindAllStringSubmatch(expr, -1)

	metricNames := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			name := match[1]
			// Filter out PromQL keywords and functions
			if !isPromQLKeyword(name) {
				metricNames[name] = true
			}
		}
	}

	result := make([]string, 0, len(metricNames))
	for name := range metricNames {
		result = append(result, name)
	}
	return result
}

// isPromQLKeyword checks if a string is a PromQL keyword or function
func isPromQLKeyword(s string) bool {
	keywords := []string{
		// Aggregation operators
		"sum", "min", "max", "avg", "group", "stddev", "stdvar", "count", "count_values",
		"bottomk", "topk", "quantile",
		// Binary operators
		"and", "or", "unless",
		// Clauses
		"by", "without", "on", "ignoring", "group_left", "group_right",
		// Functions
		"rate", "irate", "increase", "delta", "idelta", "deriv", "predict_linear",
		"histogram_quantile", "label_replace", "label_join", "ln", "log2", "log10",
		"abs", "ceil", "floor", "round", "exp", "sqrt", "time", "vector", "scalar",
		"sort", "sort_desc", "timestamp", "absent", "absent_over_time",
		"changes", "clamp_max", "clamp_min", "day_of_month", "day_of_week",
		"days_in_month", "hour", "minute", "month", "year", "resets",
		// Aggregation over time
		"avg_over_time", "min_over_time", "max_over_time", "sum_over_time",
		"count_over_time", "quantile_over_time", "stddev_over_time", "stdvar_over_time",
		"last_over_time", "present_over_time",
	}

	lower := strings.ToLower(s)
	for _, kw := range keywords {
		if lower == kw {
			return true
		}
	}
	return false
}

// checkMetricNamingConventions checks if metric names follow Prometheus naming conventions
func checkMetricNamingConventions(metricName string) []string {
	var issues []string

	// Skip standard Prometheus internal metrics
	standardMetrics := []string{"up", "scrape_duration_seconds", "scrape_samples_scraped",
		"scrape_samples_post_metric_relabeling", "scrape_series_added"}
	for _, std := range standardMetrics {
		if metricName == std {
			return issues
		}
	}

	// Check for invalid characters (must be [a-zA-Z_:][a-zA-Z0-9_:]*)
	if !regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`).MatchString(metricName) {
		issues = append(issues, fmt.Sprintf("Metric '%s' contains invalid characters (must match [a-zA-Z_:][a-zA-Z0-9_:]*)", metricName))
	}

	// Check if metric name uses snake_case (no camelCase)
	if regexp.MustCompile(`[a-z][A-Z]`).MatchString(metricName) {
		issues = append(issues, fmt.Sprintf("Metric '%s' should use snake_case, not camelCase", metricName))
	}

	// Check for application prefix (should have at least one underscore suggesting a namespace)
	if !strings.Contains(metricName, "_") && !strings.Contains(metricName, ":") {
		issues = append(issues, fmt.Sprintf("Metric '%s' should have an application prefix (e.g., 'myapp_%s')", metricName, metricName))
	}

	return issues
}

// checkMetricSuffixes validates that metrics use proper unit suffixes
func checkMetricSuffixes(metricName string) []string {
	var issues []string

	// Known counter patterns that should have _total suffix
	if isCounterPattern(metricName) && !strings.HasSuffix(metricName, "_total") {
		issues = append(issues, fmt.Sprintf("Counter metric '%s' should have '_total' suffix", metricName))
	}

	// Check for non-base units
	nonBaseUnits := map[string]string{
		"_milliseconds": "_seconds",
		"_microseconds": "_seconds",
		"_nanoseconds":  "_seconds",
		"_minutes":      "_seconds",
		"_hours":        "_seconds",
		"_days":         "_seconds",
		"_kilobytes":    "_bytes",
		"_megabytes":    "_bytes",
		"_gigabytes":    "_bytes",
		"_terabytes":    "_bytes",
		"_millis":       "_seconds",
		"_micros":       "_seconds",
		"_nanos":        "_seconds",
		"_kb":           "_bytes",
		"_mb":           "_bytes",
		"_gb":           "_bytes",
		"_tb":           "_bytes",
		"_ms":           "_seconds",
		"_us":           "_seconds",
		"_ns":           "_seconds",
	}

	for nonBase, base := range nonBaseUnits {
		if strings.HasSuffix(metricName, nonBase) {
			issues = append(issues, fmt.Sprintf("Metric '%s' should use base unit '%s' instead of '%s'", metricName, base, nonBase))
		}
	}

	// Check for percentage - should use _ratio suffix (0-1) instead of _percent (0-100)
	if strings.Contains(metricName, "_percent") || strings.Contains(metricName, "_percentage") {
		issues = append(issues, fmt.Sprintf("Metric '%s' should use '_ratio' suffix with values 0-1 instead of percentage", metricName))
	}

	return issues
}

// isCounterPattern detects if a metric name suggests it's a counter
func isCounterPattern(metricName string) bool {
	counterKeywords := []string{
		"_count", "_requests", "_errors", "_failures", "_success",
		"_processed", "_received", "_sent", "_created", "_deleted",
	}

	// Skip if already has _total
	if strings.HasSuffix(metricName, "_total") {
		return false
	}

	for _, keyword := range counterKeywords {
		if strings.Contains(metricName, keyword) {
			return true
		}
	}

	return false
}

// checkInstrumentationPatterns checks for common instrumentation anti-patterns
func checkInstrumentationPatterns(expr string) []string {
	var issues []string

	// Check for rate() applied to gauges (common mistake)
	// Look for rate() or irate() applied to metrics without _total or time-based suffixes
	rateRegex := regexp.MustCompile(`\b(rate|irate)\s*\(\s*([a-zA-Z_:][a-zA-Z0-9_:]*)\s*[\[\{]`)
	if matches := rateRegex.FindAllStringSubmatch(expr, -1); len(matches) > 0 {
		for _, match := range matches {
			if len(match) > 2 {
				metricName := match[2]
				// Only warn if it doesn't look like a counter
				if !strings.HasSuffix(metricName, "_total") &&
					!strings.HasSuffix(metricName, "_seconds_total") &&
					!strings.HasSuffix(metricName, "_count") &&
					!strings.Contains(metricName, "_seconds") {
					issues = append(issues, fmt.Sprintf("Using %s() on '%s' which may not be a counter - rate() should only be used with counters", match[1], metricName))
				}
			}
		}
	}

	// Check for division by zero protection patterns
	// Suggest using 'or' to handle division by zero
	if strings.Contains(expr, " / ") && !strings.Contains(expr, " or ") {
		if !strings.Contains(expr, "!= 0") {
			issues = append(issues, "Division detected without zero-protection - consider adding '... or 1' or checking for non-zero denominator")
		}
	}

	return issues
}
