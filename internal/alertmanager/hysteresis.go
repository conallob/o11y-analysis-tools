// Package alertmanager provides utilities for analyzing Alertmanager alert firing patterns and hysteresis.
package alertmanager

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// HysteresisAnalyzer analyzes alert firing patterns
type HysteresisAnalyzer struct {
	prometheusURL string
	verbose       bool
}

// AlertEvent represents a single alert firing event
type AlertEvent struct {
	AlertName string
	StartsAt  time.Time
	EndsAt    time.Time
	Duration  time.Duration
	Labels    map[string]string
}

// AlertAnalysis contains the analysis results for an alert
type AlertAnalysis struct {
	AlertName      string
	FiringCount    int
	AvgDuration    time.Duration
	MedianDuration time.Duration
	MinDuration    time.Duration
	MaxDuration    time.Duration
	RecommendedFor time.Duration
	SpuriousAlerts int
	Reasoning      string
}

// PrometheusResponse represents the Prometheus API response
type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// NewHysteresisAnalyzer creates a new analyzer
func NewHysteresisAnalyzer(prometheusURL string, verbose bool) *HysteresisAnalyzer {
	return &HysteresisAnalyzer{
		prometheusURL: prometheusURL,
		verbose:       verbose,
	}
}

// FetchAlertHistory fetches alert firing history from Prometheus
func (a *HysteresisAnalyzer) FetchAlertHistory(timeframe time.Duration, alertName string) (map[string][]AlertEvent, error) {
	// Query for ALERTS metric which tracks firing alerts
	query := "ALERTS"
	if alertName != "" {
		query = fmt.Sprintf(`ALERTS{alertname="%s"}`, alertName)
	}

	// Build query URL
	endTime := time.Now()
	startTime := endTime.Add(-timeframe)

	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", startTime.Unix()))
	params.Add("end", fmt.Sprintf("%d", endTime.Unix()))
	params.Add("step", "60s") // 1 minute resolution

	queryURL := fmt.Sprintf("%s/api/v1/query_range?%s", a.prometheusURL, params.Encode())

	if a.verbose {
		fmt.Printf("Query URL: %s\n", queryURL)
	}

	// Make HTTP request
	resp, err := http.Get(queryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			}
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var promResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Process results into alert events
	events := make(map[string][]AlertEvent)

	for _, result := range promResp.Data.Result {
		alertName := result.Metric["alertname"]
		if alertName == "" {
			continue
		}

		// Convert time series data into discrete firing events
		var currentEvent *AlertEvent

		for _, value := range result.Values {
			timestamp := int64(value[0].(float64))
			valueStr := value[1].(string)

			// Value "1" means alert is firing
			if valueStr == "1" {
				if currentEvent == nil {
					// Start of new firing event
					currentEvent = &AlertEvent{
						AlertName: alertName,
						StartsAt:  time.Unix(timestamp, 0),
						Labels:    result.Metric,
					}
				}
				// Update end time as long as alert is firing
				currentEvent.EndsAt = time.Unix(timestamp, 0)
			} else if currentEvent != nil {
				// Alert stopped firing
				currentEvent.Duration = currentEvent.EndsAt.Sub(currentEvent.StartsAt)
				events[alertName] = append(events[alertName], *currentEvent)
				currentEvent = nil
			}
		}

		// Handle case where alert is still firing
		if currentEvent != nil {
			currentEvent.EndsAt = time.Now()
			currentEvent.Duration = currentEvent.EndsAt.Sub(currentEvent.StartsAt)
			events[alertName] = append(events[alertName], *currentEvent)
		}
	}

	return events, nil
}

// AnalyzeAlert analyzes alert firing patterns and recommends a 'for' duration
func (a *HysteresisAnalyzer) AnalyzeAlert(alertName string, events []AlertEvent) AlertAnalysis {
	analysis := AlertAnalysis{
		AlertName:   alertName,
		FiringCount: len(events),
	}

	if len(events) == 0 {
		return analysis
	}

	// Calculate duration statistics
	durations := make([]time.Duration, len(events))
	var totalDuration time.Duration

	for i, event := range events {
		durations[i] = event.Duration
		totalDuration += event.Duration

		if analysis.MinDuration == 0 || event.Duration < analysis.MinDuration {
			analysis.MinDuration = event.Duration
		}
		if event.Duration > analysis.MaxDuration {
			analysis.MaxDuration = event.Duration
		}
	}

	analysis.AvgDuration = totalDuration / time.Duration(len(events))

	// Calculate median
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	if len(durations)%2 == 0 {
		mid := len(durations) / 2
		analysis.MedianDuration = (durations[mid-1] + durations[mid]) / 2
	} else {
		analysis.MedianDuration = durations[len(durations)/2]
	}

	// Recommend 'for' duration based on analysis
	// Strategy: Use a percentile approach to filter out spurious short-lived alerts
	// Recommend the duration that would filter out the shortest 30% of alerts

	percentile30 := int(float64(len(durations)) * 0.3)
	if percentile30 >= len(durations) {
		percentile30 = len(durations) - 1
	}

	recommended := durations[percentile30]

	// Round to sensible values (30s, 1m, 2m, 5m, 10m, 15m, 30m, 1h)
	recommended = roundToSensibleDuration(recommended)

	analysis.RecommendedFor = recommended

	// Count spurious alerts (those shorter than recommended)
	for _, d := range durations {
		if d < recommended {
			analysis.SpuriousAlerts++
		}
	}

	// Generate reasoning
	if analysis.SpuriousAlerts > 0 {
		percentage := float64(analysis.SpuriousAlerts) / float64(len(events)) * 100
		analysis.Reasoning = fmt.Sprintf(
			"%.1f%% of alerts (%d/%d) fire for less than %s, suggesting spurious alerts",
			percentage, analysis.SpuriousAlerts, len(events), recommended.Round(time.Second))
	} else {
		analysis.Reasoning = "All alerts fire for longer than the recommended duration"
	}

	return analysis
}

// roundToSensibleDuration rounds a duration to sensible alert 'for' values
func roundToSensibleDuration(d time.Duration) time.Duration {
	sensibleDurations := []time.Duration{
		30 * time.Second,
		1 * time.Minute,
		2 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
		6 * time.Hour,
		12 * time.Hour,
		24 * time.Hour,
	}

	for _, sd := range sensibleDurations {
		if d <= sd {
			return sd
		}
	}

	return sensibleDurations[len(sensibleDurations)-1]
}

// PrometheusRuleGroup represents a Prometheus rule group
type PrometheusRuleGroup struct {
	Name  string `yaml:"name"`
	Rules []struct {
		Alert string `yaml:"alert"`
		For   string `yaml:"for"`
	} `yaml:"rules"`
}

// PrometheusRules represents the top-level Prometheus rules structure
type PrometheusRules struct {
	Groups []PrometheusRuleGroup `yaml:"groups"`
}

// LoadAlertDurations loads configured 'for' durations from a Prometheus rules file
func LoadAlertDurations(filename string) (map[string]time.Duration, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var rules PrometheusRules
	if err := yaml.Unmarshal(content, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	durations := make(map[string]time.Duration)

	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			if rule.Alert != "" && rule.For != "" {
				duration, err := time.ParseDuration(rule.For)
				if err != nil {
					// Try parsing without 's' suffix (Prometheus allows '5m' or '5m0s')
					duration, err = time.ParseDuration(rule.For + "0s")
					if err != nil {
						continue
					}
				}
				durations[rule.Alert] = duration
			}
		}
	}

	return durations, nil
}
