package alertmanager

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSplitTimeRange(t *testing.T) {
	start := time.Unix(0, 0)
	tests := []struct {
		name      string
		end       time.Time
		step      time.Duration
		maxPoints int
		wantWins  int
	}{
		{"fits in one page", start.Add(9 * time.Minute), time.Minute, 10, 1},
		{"exactly two pages", start.Add(19 * time.Minute), time.Minute, 10, 2},
		{"partial last page", start.Add(25 * time.Minute), time.Minute, 10, 3},
		{"zero max points clamps to one", start.Add(2 * time.Minute), time.Minute, 0, 3},
		{"empty range still one page", start, time.Minute, 10, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wins := splitTimeRange(start, tt.end, tt.step, tt.maxPoints)
			if len(wins) != tt.wantWins {
				t.Fatalf("got %d windows, want %d: %v", len(wins), tt.wantWins, wins)
			}
			if !wins[0].start.Equal(start) || !wins[len(wins)-1].end.Equal(tt.end) {
				t.Errorf("windows do not cover range: %v", wins)
			}
			limit := tt.maxPoints
			if limit < 1 {
				limit = 1
			}
			for i, w := range wins {
				points := int(w.end.Sub(w.start)/tt.step) + 1
				if points > limit {
					t.Errorf("window %d has %d points, limit %d", i, points, limit)
				}
				if i > 0 && w.start != wins[i-1].end.Add(tt.step) {
					t.Errorf("window %d not contiguous with previous", i)
				}
			}
		})
	}
}

func TestPaginationOptionsDefault(t *testing.T) {
	if got := (PaginationOptions{}).maxPoints(); got != DefaultMaxPointsPerQuery {
		t.Errorf("default = %d, want %d", got, DefaultMaxPointsPerQuery)
	}
	if got := (PaginationOptions{MaxPointsPerQuery: 5}).maxPoints(); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

// pagedServer returns a firing sample at every step within each requested window.
func pagedServer(t *testing.T, mu *sync.Mutex, requests *[][2]int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		end, _ := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
		step, _ := strconv.ParseInt(r.URL.Query().Get("step")[:len(r.URL.Query().Get("step"))-1], 10, 64)
		mu.Lock()
		*requests = append(*requests, [2]int64{start, end})
		mu.Unlock()

		values := ""
		for ts := start; ts <= end; ts += step {
			if values != "" {
				values += ","
			}
			values += fmt.Sprintf(`[%d,"1"]`, ts)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"alertname":"A"},"values":[%s]}]}}`, values)
	}))
}

func TestFetchAlertHistoryPaginated(t *testing.T) {
	var mu sync.Mutex
	var reqs [][2]int64
	srv := pagedServer(t, &mu, &reqs)
	defer srv.Close()

	a := NewHysteresisAnalyzer(srv.URL, false)
	a.SetPagination(PaginationOptions{MaxPointsPerQuery: 10})

	events, err := a.FetchAlertHistory(45*time.Minute, "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) < 4 {
		t.Fatalf("expected multiple paged requests, got %d", len(reqs))
	}
	for _, r := range reqs {
		if points := (r[1]-r[0])/60 + 1; points > 10 {
			t.Errorf("request %v exceeds 10 points (%d)", r, points)
		}
	}
	// Continuous firing across page boundaries must be stitched into ONE event.
	if len(events["A"]) != 1 {
		t.Fatalf("expected 1 stitched event, got %d", len(events["A"]))
	}
}

func TestFindLastFiredTimesPaged(t *testing.T) {
	var mu sync.Mutex
	var reqs [][2]int64
	srv := pagedServer(t, &mu, &reqs)
	defer srv.Close()

	start := time.Now()
	got, err := FindLastFiredTimesPaged(srv.URL, []string{"A", "B"}, 100*time.Hour, false,
		PaginationOptions{MaxPointsPerQuery: 30, Delay: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 4 { // 101 hourly points / 30 per page
		t.Errorf("expected 4 requests, got %d", len(reqs))
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Errorf("delay between pages not applied")
	}
	if got["A"].IsZero() {
		t.Error("A should have a last-fired time from the final page")
	}
	if !got["B"].IsZero() {
		t.Error("B never fired and should be zero")
	}
}

func TestPagedQueryErrorNamesPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 2 {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	defer srv.Close()

	_, err := FindLastFiredTimesPaged(srv.URL, []string{"A"}, 100*time.Hour, false, PaginationOptions{MaxPointsPerQuery: 30})
	if err == nil {
		t.Fatal("expected error")
	}
	if want := "page 2/"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should mention %q", err, want)
	}
}
