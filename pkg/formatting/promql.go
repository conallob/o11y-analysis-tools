// Package formatting provides utilities for formatting and validating PromQL expressions.
package formatting

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CheckOptions configures the behavior of CheckAndFormatPromQL
type CheckOptions struct {
	DisableLineLength bool
	PrometheusURL     string
	Verbose           bool
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

// PromQLRule represents a single Prometheus rule (alert or recording)
type PromQLRule struct {
	Alert       string            `yaml:"alert,omitempty"`
	Record      string            `yaml:"record,omitempty"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// PrometheusRuleGroup represents a Prometheus rule group
type PrometheusRuleGroup struct {
	Name     string       `yaml:"name"`
	Interval string       `yaml:"interval,omitempty"`
	Rules    []PromQLRule `yaml:"rules"`
}

// PrometheusRules represents the top-level Prometheus rules structure
type PrometheusRules struct {
	Groups []PrometheusRuleGroup `yaml:"groups"`
}

// CheckAndFormatPromQL analyzes YAML content for PromQL expressions and formats them
func CheckAndFormatPromQL(content string, opts CheckOptions) ([]string, string) {
	var issues []string
	formatted := content

	// Check for alert rules with both duration and hysteresis
	hysteresisIssues := checkAlertHysteresisWithDuration(content)
	issues = append(issues, hysteresisIssues...)

	// Check timeseries continuity if Prometheus URL provided
	if opts.PrometheusURL != "" {
		continuityIssues := checkTimeseriesContinuity(content, opts.PrometheusURL, opts.Verbose)
		issues = append(issues, continuityIssues...)
	}

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

		// Check for redundant aggregation clauses
		redundantIssues := checkRedundantAggregations(expression)
		issues = append(issues, redundantIssues...)

		// Check for aggregation placement
		placementIssues := checkAggregationPlacement(expression)
		issues = append(issues, placementIssues...)

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
	// Formatting rules:
	// 1. Split by binary operators (/, *, +, -, etc.)
	// 2. Each operand on its own line(s)
	// 3. Binary operators indented by 2 spaces on their own line
	// 4. Nested expressions properly indented
	// 5. Remove redundant aggregation clauses from left operand when both operands have the same clause
	// 6. Add on() clause when operands have common label selectors for explicit vector matching

	// First, try to split by binary operators
	binaryOps := []string{" / ", " * ", " + ", " - ", " % ", " ^ "}

	for _, op := range binaryOps {
		if strings.Contains(expr, op) {
			parts := splitByBinaryOperator(expr, op)
			if len(parts) == 2 {
				leftStr := strings.TrimSpace(parts[0])
				rightStr := strings.TrimSpace(parts[1])

				// Check if both operands have the same aggregation clause
				leftAgg := extractTrailingAggregation(leftStr)
				rightAgg := extractTrailingAggregation(rightStr)

				// If both have the same aggregation (and it's not 'without'), omit from left
				// Exception: 'without' needs to be explicit on both sides
				omitLeftAggregation := leftAgg != "" && leftAgg == rightAgg && !strings.Contains(leftAgg, "without")

				// Extract matching labels from aggregation clause if present
				var matchingLabels []string
				if rightAgg != "" && strings.Contains(rightAgg, "by") {
					// Extract labels from by clause: ) by (label1, label2)
					byRegex := regexp.MustCompile(`by\s*\(([^)]+)\)`)
					if match := byRegex.FindStringSubmatch(rightAgg); len(match) >= 2 {
						labelStr := strings.TrimSpace(match[1])
						labels := strings.Split(labelStr, ",")
						for _, label := range labels {
							matchingLabels = append(matchingLabels, strings.TrimSpace(label))
						}
					}
				}

				// Format each operand
				left := formatOperand(leftStr, 0, omitLeftAggregation)
				right := formatOperand(rightStr, 0, false)

				// Build operator line with optional on() clause
				opLine := strings.TrimSpace(op)
				if len(matchingLabels) > 0 {
					// Add on() clause for explicit vector matching
					opLine = opLine + " on (" + strings.Join(matchingLabels, ", ") + ")"
				}

				// Combine with indented operator
				return left + "\n  " + opLine + "\n" + right
			}
		}
	}

	// If no binary operators, format as a single operand
	return formatOperand(expr, 0, false)
}

// splitByBinaryOperator splits expression by a binary operator, respecting parentheses
func splitByBinaryOperator(expr, op string) []string {
	depth := 0
	inQuote := false
	var quoteChar rune

	for i := 0; i < len(expr); i++ {
		ch := rune(expr[i])

		// Handle quotes
		if (ch == '"' || ch == '\'') && (i == 0 || expr[i-1] != '\\') {
			if !inQuote {
				inQuote = true
				quoteChar = ch
			} else if ch == quoteChar {
				inQuote = false
			}
			continue
		}

		if inQuote {
			continue
		}

		// Track parentheses depth
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
		}

		// Only split at top level (depth 0)
		if depth == 0 && i+len(op) <= len(expr) {
			if expr[i:i+len(op)] == op {
				return []string{expr[:i], expr[i+len(op):]}
			}
		}
	}

	return []string{expr}
}

// formatOperand formats a single operand (which may be an aggregation with nested expressions)
// If omitAggregation is true, any trailing aggregation clause (by/without) will be omitted
func formatOperand(expr string, baseIndent int, omitAggregation bool) string {
	expr = strings.TrimSpace(expr)

	// Check if this is an aggregation with prefix style: sum by (labels) (expr)
	// Pattern: aggregation_func [by/without (labels)] (expr)
	aggOps := []string{"sum", "avg", "min", "max", "count", "stddev", "stdvar", "topk", "bottomk", "quantile", "count_values"}

	for _, aggOp := range aggOps {
		// First, check for postfix style: sum(...) by (labels)
		// This is the most common style we need to reformat to prefix
		postfixPattern := regexp.MustCompile(`^(` + aggOp + `)\s*(\([^)]+(?:\([^)]*\))*[^)]*\))\s+(by|without)\s*(\([^)]+\))$`)
		if matches := postfixPattern.FindStringSubmatch(expr); matches != nil {
			aggFunc := matches[1]
			innerExpr := matches[2]
			byOrWithout := matches[3]
			labels := matches[4]

			// Remove outer parentheses from inner expression
			innerExpr = strings.TrimSpace(innerExpr)
			if strings.HasPrefix(innerExpr, "(") && strings.HasSuffix(innerExpr, ")") {
				innerExpr = innerExpr[1 : len(innerExpr)-1]
				innerExpr = strings.TrimSpace(innerExpr)
			}

			// Format the inner expression with indentation
			indent := strings.Repeat(" ", baseIndent+2)
			formattedInner := indent + innerExpr

			// If omitAggregation is true, format without the by/without clause
			if omitAggregation {
				return fmt.Sprintf("%s (\n%s\n%s)",
					aggFunc, formattedInner, strings.Repeat(" ", baseIndent))
			}

			return fmt.Sprintf("%s %s %s (\n%s\n%s)",
				aggFunc, byOrWithout, labels, formattedInner, strings.Repeat(" ", baseIndent))
		}

		// Check for prefix style: sum by (labels) (expr)
		prefixPattern := regexp.MustCompile(`^(` + aggOp + `)\s+(by|without)\s*(\([^)]+\))\s*(\(.+\))$`)
		if matches := prefixPattern.FindStringSubmatch(expr); matches != nil {
			// Already in prefix style, just format with proper indentation
			aggFunc := matches[1]
			byOrWithout := matches[2]
			labels := matches[3]
			innerExpr := matches[4]

			// Remove outer parentheses from inner expression
			innerExpr = strings.TrimSpace(innerExpr)
			if strings.HasPrefix(innerExpr, "(") && strings.HasSuffix(innerExpr, ")") {
				innerExpr = innerExpr[1 : len(innerExpr)-1]
				innerExpr = strings.TrimSpace(innerExpr)
			}

			// Format the inner expression with indentation
			indent := strings.Repeat(" ", baseIndent+2)
			formattedInner := indent + innerExpr

			// If omitAggregation is true, format without the by/without clause
			if omitAggregation {
				return fmt.Sprintf("%s (\n%s\n%s)",
					aggFunc, formattedInner, strings.Repeat(" ", baseIndent))
			}

			return fmt.Sprintf("%s %s %s (\n%s\n%s)",
				aggFunc, byOrWithout, labels, formattedInner, strings.Repeat(" ", baseIndent))
		}

		// Also check for simple aggregation without by/without: sum(expr)
		simplePattern := regexp.MustCompile(`^(` + aggOp + `)\s*(\(.+\))$`)
		if matches := simplePattern.FindStringSubmatch(expr); matches != nil {
			aggFunc := matches[1]
			innerExpr := matches[2]

			// Remove outer parentheses
			innerExpr = strings.TrimSpace(innerExpr)
			if strings.HasPrefix(innerExpr, "(") && strings.HasSuffix(innerExpr, ")") {
				innerExpr = innerExpr[1 : len(innerExpr)-1]
				innerExpr = strings.TrimSpace(innerExpr)
			}

			// Always format with multiline for consistency in binary operations
			indent := strings.Repeat(" ", baseIndent+2)
			formattedInner := indent + innerExpr
			return fmt.Sprintf("%s (\n%s\n%s)",
				aggFunc, formattedInner, strings.Repeat(" ", baseIndent))
		}
	}

	// No special formatting needed
	return expr
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

		// Check recording rule naming (if applicable)
		issues = append(issues, checkRecordingRuleNaming(metricName)...)
	}

	// Check variable/metric naming conventions
	issues = append(issues, checkVariableNaming(expr)...)

	// Check label naming conventions
	issues = append(issues, checkLabelNaming(expr)...)

	// Check for instrumentation best practices
	issues = append(issues, checkInstrumentationPatterns(expr)...)

	// Check for utilization metrics without proper total divisor
	issues = append(issues, checkUtilizationDivisor(expr)...)

	// Check for synthetic metrics without proper label selectors
	issues = append(issues, checkSyntheticMetrics(expr)...)

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

// checkUtilizationDivisor validates that utilization metrics are divided by a total metric
// Utilization metrics should follow the pattern: used / total
// The denominator (second operand of division) should contain "_total" or "total" in the metric name
func checkUtilizationDivisor(expr string) []string {
	var issues []string

	// Check if the expression involves division
	if !strings.Contains(expr, " / ") {
		return issues
	}

	// Extract metric names from the expression to check if any indicate utilization
	metricNames := extractMetricNames(expr)
	hasUtilization := false
	for _, name := range metricNames {
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "utilization") {
			hasUtilization = true
			break
		}
	}

	// If no utilization metrics found, nothing to check
	if !hasUtilization {
		return issues
	}

	// Split by division operator, respecting parentheses
	parts := splitByBinaryOperator(expr, " / ")
	if len(parts) != 2 {
		return issues
	}

	// Extract the denominator (right side of division)
	denominator := strings.TrimSpace(parts[1])

	// Extract metric names from the denominator
	denominatorMetrics := extractMetricNames(denominator)

	// Check if any metric in the denominator has "total" or "_total"
	hasTotal := false
	for _, metric := range denominatorMetrics {
		metricLower := strings.ToLower(metric)
		if strings.Contains(metricLower, "_total") || strings.HasSuffix(metricLower, "total") {
			hasTotal = true
			break
		}
	}

	if !hasTotal {
		issues = append(issues, fmt.Sprintf(
			"Utilization metric detected but denominator does not contain a 'total' metric - "+
				"utilization should be calculated as (used / total), where the denominator metric name contains '_total' or 'total'"))
	}

	return issues
}

// checkSyntheticMetrics validates that synthetic metrics have proper label selectors
func checkSyntheticMetrics(expr string) []string {
	var issues []string

	// Check for 'up' metric without job label selector
	// Pattern: match 'up' as a word boundary, optionally followed by label selectors,
	// then followed by a valid boundary character (bracket, paren, whitespace, comma, or end)
	upRegex := regexp.MustCompile(`\bup(?:\s*(\{[^}]*\}))?(?:\s*\[|[\)\s,]|$)`)
	matches := upRegex.FindAllStringSubmatch(expr, -1)

	for _, match := range matches {
		hasJobLabel := false

		// Check if there's a label selector (captured in match[1])
		if len(match) > 1 && match[1] != "" {
			labelSelector := match[1]
			// Look for job="..." or job=~"..."
			if regexp.MustCompile(`job\s*=~?`).MatchString(labelSelector) {
				hasJobLabel = true
			}
		}

		if !hasJobLabel {
			issues = append(issues, "Synthetic metric 'up' should always include a job label selector (e.g., up{job=\"...\"}) to avoid matching multiple jobs")
		}
	}

	return issues
}

// checkVariableNaming validates metric/variable names according to Prometheus naming conventions
func checkVariableNaming(expr string) []string {
	var issues []string

	// Extract metric names from the expression
	metricNames := extractMetricNames(expr)

	for _, metricName := range metricNames {
		// Skip if it's already checked by checkMetricNamingConventions
		// This function focuses on additional variable naming rules

		// Check 1: Metric names should match [a-zA-Z_:][a-zA-Z0-9_:]*
		validMetricRegex := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
		if !validMetricRegex.MatchString(metricName) {
			issues = append(issues, fmt.Sprintf("Metric name '%s' should only contain alphanumeric characters, underscores, and colons, and must not start with a digit", metricName))
			continue
		}

		// Check 2: Avoid colons in metric names (reserved for recording rules)
		// Only check if it's not a recording rule format (level:metric:operations)
		if strings.Contains(metricName, ":") {
			// Check if it follows the recording rule format
			parts := strings.Split(metricName, ":")
			if len(parts) < 2 {
				issues = append(issues, fmt.Sprintf("Metric name '%s' should not contain colons unless it's a recording rule (format: level:metric:operations)", metricName))
			}
			// If it has colons, we'll validate it with checkRecordingRuleNaming
		}

		// Check 3: Metric names should use lowercase and underscores (snake_case)
		hasUppercase := false
		for _, char := range metricName {
			if char >= 'A' && char <= 'Z' {
				hasUppercase = true
				break
			}
		}
		if hasUppercase {
			issues = append(issues, fmt.Sprintf("Metric name '%s' should use lowercase with underscores (snake_case), not camelCase or PascalCase", metricName))
		}

		// Check 4: Don't put metric type in the name
		metricTypes := []string{"_gauge", "_counter", "_summary", "_histogram"}
		for _, metricType := range metricTypes {
			if strings.HasSuffix(metricName, metricType) {
				issues = append(issues, fmt.Sprintf("Metric name '%s' should not include the metric type (%s) in the name", metricName, metricType))
			}
		}
	}

	return issues
}

// checkLabelNaming validates label names according to Prometheus naming conventions
func checkLabelNaming(expr string) []string {
	var issues []string

	// Extract label names from label selectors {label="value"}
	labelRegex := regexp.MustCompile(`\{([^}]+)\}`)
	matches := labelRegex.FindAllStringSubmatch(expr, -1)

	seenLabels := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		labelSelector := match[1]
		// Parse individual label matchers (label="value", label=~"regex", etc.)
		labelPairs := strings.Split(labelSelector, ",")

		for _, pair := range labelPairs {
			pair = strings.TrimSpace(pair)
			// Extract label name (before = or =~ or != or !~)
			labelNameRegex := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)(?:\s*[!=]~?\s*.+)?$`)
			labelMatch := labelNameRegex.FindStringSubmatch(pair)

			if len(labelMatch) < 2 {
				continue
			}

			labelName := labelMatch[1]

			// Skip if we've already checked this label
			if seenLabels[labelName] {
				continue
			}
			seenLabels[labelName] = true

			// Check 1: Label names should match [a-zA-Z_][a-zA-Z0-9_]*
			validLabelRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
			if !validLabelRegex.MatchString(labelName) {
				issues = append(issues, fmt.Sprintf("Label name '%s' should only contain alphanumeric characters and underscores, and must not start with a digit", labelName))
				continue
			}

			// Check 2: Don't use leading underscores (reserved for internal use)
			if strings.HasPrefix(labelName, "__") {
				issues = append(issues, fmt.Sprintf("Label name '%s' uses double leading underscores which are reserved for internal Prometheus use", labelName))
			} else if strings.HasPrefix(labelName, "_") {
				issues = append(issues, fmt.Sprintf("Label name '%s' should not start with an underscore (reserved for internal use)", labelName))
			}

			// Check 3: Avoid generic label names that are too common
			genericLabels := []string{"type"}
			for _, generic := range genericLabels {
				if labelName == generic {
					issues = append(issues, fmt.Sprintf("Label name '%s' is too generic and should be avoided. Consider using a more specific name", labelName))
				}
			}
		}
	}

	return issues
}

// checkRecordingRuleNaming validates recording rule names follow the level:metric:operations format
func checkRecordingRuleNaming(metricName string) []string {
	var issues []string

	// Recording rules should follow the format: level:metric:operations
	// Example: job:http_requests_total:rate5m

	// Only validate if the metric name contains colons (indicating it's likely a recording rule)
	if !strings.Contains(metricName, ":") {
		return issues
	}

	parts := strings.Split(metricName, ":")

	// Should have at least 2 parts (level:metric) but typically 3 (level:metric:operations)
	if len(parts) < 2 {
		issues = append(issues, fmt.Sprintf("Recording rule '%s' should follow format 'level:metric:operations' (e.g., 'job:http_requests_total:rate5m')", metricName))
		return issues
	}

	level := parts[0]
	metric := parts[1]

	// Validate level (aggregation level) - should be label names
	// Common levels: job, instance, job_instance, cluster, etc.
	if level == "" {
		issues = append(issues, fmt.Sprintf("Recording rule '%s' has empty level component. Level should represent aggregation labels (e.g., 'job', 'instance')", metricName))
	}

	// Validate metric name component
	if metric == "" {
		issues = append(issues, fmt.Sprintf("Recording rule '%s' has empty metric component", metricName))
	}

	// The metric component should preserve the original metric name
	// Check if it's using snake_case
	hasUppercase := false
	for _, char := range metric {
		if char >= 'A' && char <= 'Z' {
			hasUppercase = true
			break
		}
	}
	if hasUppercase {
		issues = append(issues, fmt.Sprintf("Recording rule '%s' metric component should use snake_case, not camelCase", metricName))
	}

	// If there's an operations component, validate it
	if len(parts) >= 3 {
		operations := parts[2]
		if operations == "" {
			issues = append(issues, fmt.Sprintf("Recording rule '%s' has empty operations component. Operations should describe transformations (e.g., 'rate5m', 'sum')", metricName))
		}

		// Operations should describe what was done to the metric
		// Common patterns: rate5m, sum, avg, etc.
		// Should not contain spaces or special characters other than underscores
		validOperationsRegex := regexp.MustCompile(`^[a-z0-9_]+$`)
		if !validOperationsRegex.MatchString(operations) {
			issues = append(issues, fmt.Sprintf("Recording rule '%s' operations component should only contain lowercase letters, digits, and underscores", metricName))
		}

		// Check for ambiguous operation suffixes that should be avoided
		if operations == "value" {
			issues = append(issues, fmt.Sprintf("Recording rule '%s' should not use 'value' as operations component (discouraged for being ambiguous and redundant)", metricName))
		}
		if operations == "avg" {
			issues = append(issues, fmt.Sprintf("Recording rule '%s' should not use 'avg' alone (discouraged for being ambiguous - specify time window, e.g., 'avg5m')", metricName))
		}
	}

	// Validate that _total suffix is stripped when using rate() or irate()
	// This is a soft recommendation - we check if the metric ends with _total and operations suggest rate
	if strings.Contains(metric, "_total") && len(parts) >= 3 {
		operations := parts[2]
		if strings.Contains(operations, "rate") || strings.Contains(operations, "irate") {
			issues = append(issues, fmt.Sprintf("Recording rule '%s' should strip '_total' suffix from counter metrics when using rate() or irate() (expected: '%s:%s:%s')",
				metricName, level, strings.TrimSuffix(metric, "_total"), parts[2]))
		}
	}

	return issues
}

// checkRedundantAggregations detects redundant aggregation clauses in binary operations
func checkRedundantAggregations(expr string) []string {
	var issues []string

	// Look for binary operations (/, *, +, -, etc.) where both sides have the same aggregation clause
	// Example: sum(...) by (instance) / sum(...) by (instance)
	// This should be: sum(...) / sum(...) by (instance)

	// Find binary operators between aggregations
	binaryOps := []string{" / ", " * ", " + ", " - ", " % ", " ^ "}

	for _, op := range binaryOps {
		if !strings.Contains(expr, op) {
			continue
		}

		// Split by the operator
		parts := strings.Split(expr, op)
		if len(parts) != 2 {
			continue
		}

		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])

		// Extract aggregation clause from left side (looking for trailing by/without)
		leftAggClause := extractTrailingAggregation(left)
		if leftAggClause == "" {
			continue
		}

		// Extract aggregation clause from right side (looking for trailing by/without)
		rightAggClause := extractTrailingAggregation(right)
		if rightAggClause == "" {
			continue
		}

		// If both sides have the same aggregation clause, it's redundant on the left
		if leftAggClause == rightAggClause {
			issues = append(issues, fmt.Sprintf("Redundant aggregation clause '%s' on left side of '%s' - only specify on the final operand", leftAggClause, op))
		}
	}

	return issues
}

// extractTrailingAggregation extracts the trailing by/without clause from an expression
func extractTrailingAggregation(expr string) string {
	// Match patterns like: ) by (label1, label2) or ) without (label1)
	aggRegex := regexp.MustCompile(`\)\s+(by|without)\s*\([^)]+\)\s*$`)
	match := aggRegex.FindString(expr)
	if match != "" {
		return strings.TrimSpace(match)
	}
	return ""
}

// checkAggregationPlacement checks that aggregation clauses are on the final operand only
func checkAggregationPlacement(expr string) []string {
	var issues []string

	// Look for aggregation clauses on non-final operands in binary expressions
	// Example: sum(...) by (instance) / sum(...)
	// This is acceptable only if the right side also has an aggregation

	binaryOps := []string{" / ", " * ", " + ", " - ", " % ", " ^ "}

	for _, op := range binaryOps {
		if !strings.Contains(expr, op) {
			continue
		}

		// Check if there's a comparison operator after this binary op
		// If so, the final operand is after the comparison
		hasComparison := false
		for _, compOp := range []string{" > ", " < ", " >= ", " <= ", " == ", " != "} {
			if strings.Contains(expr, compOp) {
				hasComparison = true
				break
			}
		}

		// Split by the operator
		parts := strings.Split(expr, op)
		if len(parts) < 2 {
			continue
		}

		// Check all parts except the last one for aggregation clauses
		for i := 0; i < len(parts)-1; i++ {
			part := strings.TrimSpace(parts[i])
			aggClause := extractTrailingAggregation(part)

			// If we found an aggregation on a non-final part, check if it's redundant
			if aggClause != "" {
				// Get the next part to see if it also has an aggregation
				var nextPart string
				if i+1 < len(parts) {
					nextPart = strings.TrimSpace(parts[i+1])
					// If there's a comparison operator, split by that too
					if hasComparison {
						for _, compOp := range []string{" > ", " < ", " >= ", " <= ", " == ", " != "} {
							if strings.Contains(nextPart, compOp) {
								compParts := strings.Split(nextPart, compOp)
								if len(compParts) > 0 {
									nextPart = strings.TrimSpace(compParts[0])
								}
								break
							}
						}
					}
				}

				nextAggClause := extractTrailingAggregation(nextPart)

				// If the next part has the same aggregation or no aggregation at all,
				// the current one is likely redundant or misplaced
				if nextAggClause == "" || nextAggClause == aggClause {
					issues = append(issues, fmt.Sprintf("Aggregation clause '%s' should only appear on the final operand, not intermediate operands", aggClause))
					break
				}
			}
		}
	}

	return issues
}

// checkAlertHysteresisWithDuration checks for alert rules with both a duration in the expression and a 'for' clause
func checkAlertHysteresisWithDuration(content string) []string {
	var issues []string

	// Try to parse as Prometheus rules YAML
	var rules PrometheusRules
	if err := yaml.Unmarshal([]byte(content), &rules); err != nil {
		// Not valid Prometheus rules format, skip this check
		return issues
	}

	// Duration pattern in PromQL expressions: [5m], [1h], etc.
	durationRegex := regexp.MustCompile(`\[(\d+[smhdwy])\]`)

	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			// Only check alert rules (not recording rules)
			if rule.Alert == "" {
				continue
			}

			// Check if rule has both a 'for' clause and a duration in the expression
			if rule.For != "" {
				if durationRegex.MatchString(rule.Expr) {
					matches := durationRegex.FindAllStringSubmatch(rule.Expr, -1)
					durations := make([]string, 0, len(matches))
					for _, match := range matches {
						if len(match) > 1 {
							durations = append(durations, match[1])
						}
					}
					issues = append(issues, fmt.Sprintf(
						"Alert '%s' has both a 'for: %s' clause (hysteresis) and duration(s) %v in the expression - "+
							"consider removing the duration as the sliding window may interact poorly with hysteresis",
						rule.Alert, rule.For, durations))
				}
			}
		}
	}

	return issues
}

// checkTimeseriesContinuity checks PromQL rules against a running Prometheus for timeseries continuity
func checkTimeseriesContinuity(content string, prometheusURL string, verbose bool) []string {
	var issues []string

	// Try to parse as Prometheus rules YAML
	var rules PrometheusRules
	if err := yaml.Unmarshal([]byte(content), &rules); err != nil {
		// Not valid Prometheus rules format, skip this check
		return issues
	}

	// Extract all metric names from all rules
	metricNames := make(map[string]bool)
	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			names := extractMetricNames(rule.Expr)
			for _, name := range names {
				metricNames[name] = true
			}
		}
	}

	if len(metricNames) == 0 {
		return issues
	}

	// Check continuity for each metric
	for metricName := range metricNames {
		if verbose {
			fmt.Printf("Checking timeseries continuity for metric: %s\n", metricName)
		}

		// Query Prometheus for the last hour of data with 1-minute step
		isSparse, err := checkMetricContinuity(prometheusURL, metricName)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: Could not check metric '%s': %v\n", metricName, err)
			}
			continue
		}

		if isSparse {
			issues = append(issues, fmt.Sprintf(
				"Metric '%s' has sparse data (gaps > 2 minutes detected) - "+
					"timeseries databases don't handle sparse values well for alerting rules",
				metricName))
		}
	}

	return issues
}

// checkMetricContinuity checks if a metric has continuous data in Prometheus
func checkMetricContinuity(prometheusURL, metricName string) (isSparse bool, err error) {
	// Query for the last hour of data with 1-minute resolution
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)

	params := url.Values{}
	params.Add("query", metricName)
	params.Add("start", fmt.Sprintf("%d", startTime.Unix()))
	params.Add("end", fmt.Sprintf("%d", endTime.Unix()))
	params.Add("step", "60s") // 1 minute resolution

	queryURL := fmt.Sprintf("%s/api/v1/query_range?%s", prometheusURL, params.Encode())

	resp, err := http.Get(queryURL)
	if err != nil {
		return false, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	// If no data returned, metric doesn't exist or has no data
	if len(promResp.Data.Result) == 0 {
		return false, fmt.Errorf("no data found for metric")
	}

	// Check for gaps in the timeseries
	// We consider data "sparse" if there are gaps > 2 minutes (2x the step size)
	for _, result := range promResp.Data.Result {
		if len(result.Values) < 2 {
			// Not enough data points to determine continuity
			continue
		}

		var lastTimestamp int64
		gapCount := 0
		hasValidTimestamp := false

		for i, value := range result.Values {
			// Defensive check: ensure value has at least one element
			if len(value) < 1 {
				continue
			}

			// Safe type assertion with comma-ok idiom
			ts, ok := value[0].(float64)
			if !ok {
				return false, fmt.Errorf("unexpected timestamp type at index %d: expected float64, got %T", i, value[0])
			}
			timestamp := int64(ts)

			// Only check for gaps if we have a previous valid timestamp
			if hasValidTimestamp {
				gap := timestamp - lastTimestamp
				// Gap > 120 seconds (2 minutes) indicates sparse data
				if gap > 120 {
					gapCount++
				}
			}

			lastTimestamp = timestamp
			hasValidTimestamp = true
		}

		// If we found gaps in the data, consider it sparse
		if gapCount > 0 {
			return true, nil
		}
	}

	return false, nil
}
