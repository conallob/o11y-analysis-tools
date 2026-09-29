package alertmanager

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// DefaultMaxPointsPerQuery is the default cap on samples requested per series in a
// single query_range call. Prometheus rejects queries exceeding 11,000 points per
// series, so the default leaves some headroom.
const DefaultMaxPointsPerQuery = 10000

// PaginationOptions controls how a long time range is split into multiple
// query_range requests so no single request returns too many data points.
type PaginationOptions struct {
	// MaxPointsPerQuery is the maximum number of points per series in one request.
	// Values <= 0 use DefaultMaxPointsPerQuery.
	MaxPointsPerQuery int
	// Delay is the pause between consecutive requests, to go easy on the server.
	Delay time.Duration
}

func (o PaginationOptions) maxPoints() int {
	if o.MaxPointsPerQuery <= 0 {
		return DefaultMaxPointsPerQuery
	}
	return o.MaxPointsPerQuery
}

// timeWindow is a closed [start, end] interval covered by one page.
type timeWindow struct {
	start, end time.Time
}

// splitTimeRange splits [start, end] into consecutive non-overlapping windows that each
// contain at most maxPoints evaluation points when queried at the given step. Because
// Prometheus evaluates at start, start+step, ..., each window spans (maxPoints-1)*step
// and the next window begins one step after the previous window ends.
func splitTimeRange(start, end time.Time, step time.Duration, maxPoints int) []timeWindow {
	if maxPoints < 1 {
		maxPoints = 1
	}
	span := time.Duration(maxPoints-1) * step
	var windows []timeWindow
	for cur := start; !cur.After(end); cur = cur.Add(span + step) {
		winEnd := cur.Add(span)
		if winEnd.After(end) {
			winEnd = end
		}
		windows = append(windows, timeWindow{start: cur, end: winEnd})
	}
	return windows
}

type promSeries struct {
	Metric map[string]string
	Values [][]interface{}
}

// seriesKey builds a stable identity for a label set.
func seriesKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%q,", k, m[k])
	}
	return b.String()
}

// queryRangePaged runs query over [start, end] in windows of at most opts.maxPoints()
// points per series, and returns the series with their values stitched together in
// chronological order. onPage, if non-nil, is called before each request.
func queryRangePaged(prometheusURL, query string, start, end time.Time, step time.Duration, opts PaginationOptions, onPage func(i, total int, w timeWindow)) ([]promSeries, error) {
	windows := splitTimeRange(start, end, step, opts.maxPoints())

	var order []string
	merged := make(map[string]*promSeries)

	for i, w := range windows {
		if i > 0 && opts.Delay > 0 {
			time.Sleep(opts.Delay)
		}
		if onPage != nil {
			onPage(i, len(windows), w)
		}

		resp, err := queryRangeOnce(prometheusURL, query, w, step)
		if err != nil {
			if len(windows) > 1 {
				return nil, fmt.Errorf("page %d/%d (%s to %s): %w", i+1, len(windows),
					w.start.Format(time.RFC3339), w.end.Format(time.RFC3339), err)
			}
			return nil, err
		}

		for _, r := range resp.Data.Result {
			key := seriesKey(r.Metric)
			s, ok := merged[key]
			if !ok {
				s = &promSeries{Metric: r.Metric}
				merged[key] = s
				order = append(order, key)
			}
			s.Values = append(s.Values, r.Values...)
		}
	}

	out := make([]promSeries, 0, len(order))
	for _, key := range order {
		out = append(out, *merged[key])
	}
	return out, nil
}

func queryRangeOnce(prometheusURL, query string, w timeWindow, step time.Duration) (*PrometheusResponse, error) {
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", w.start.Unix()))
	params.Add("end", fmt.Sprintf("%d", w.end.Unix()))
	params.Add("step", fmt.Sprintf("%ds", int64(step.Seconds())))

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query_range?%s", prometheusURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var promResp PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &promResp, nil
}
