# feat: Add stale-alerts-analyzer CLI tool

## Summary

This PR introduces a new CLI tool **stale-alerts-analyzer** that helps identify and optionally delete Prometheus alerts that haven't fired within a configurable time horizon. This is useful for maintaining alert hygiene and removing outdated or unnecessary alerts from monitoring systems.

## Features

### Core Functionality

- **Alert Activity Analysis**: Queries Prometheus to determine when each alert last fired
- **Stale Alert Detection**: Identifies alerts that haven't fired within a configurable time horizon
- **Automated Cleanup**: `--fix` flag automatically removes stale alerts from Prometheus rules files
- **Detailed Reporting**: Provides comprehensive output categorizing alerts as:
  - Active (fired recently)
  - Stale (not fired within time horizon)
  - Never fired (no firing history in lookback period)

### Extended Duration Support

The tool supports an intuitive duration syntax with extended time units:

- **Standard Go units**: `h` (hours), `m` (minutes), `s` (seconds)
- **Extended units**:
  - `d` - days
  - `w` - weeks
  - `M` - months (~30 days)
  - `y` - years (~365 days)

**Default**: `12M` (12 months)

### Usage Examples

```bash
# Check for stale alerts (default 12 months)
stale-alerts-analyzer --prometheus-url=http://prometheus:9090 --rules=./alerts.yml

# Check with custom time horizon (6 months)
stale-alerts-analyzer --rules=./alerts.yml --timehorizon=6M

# Check with time horizon in days (90 days)
stale-alerts-analyzer --rules=./alerts.yml --timehorizon=90d

# Fix mode: automatically delete stale alerts
stale-alerts-analyzer --fix --rules=./alerts.yml --timehorizon=1y
```

## Implementation Details

### New Functions in `internal/alertmanager/hysteresis.go`

1. **`GetAlertNamesFromRules(filename string)`**
   - Extracts all alert names from a Prometheus rules file
   - Deduplicates alerts that appear in multiple groups

2. **`FindLastFiredTimes(prometheusURL string, alertNames []string, lookbackPeriod time.Duration, verbose bool)`**
   - Queries Prometheus `ALERTS` metric for firing history
   - Uses efficient batch querying with 1-hour resolution
   - Returns map of alert names to last firing timestamp

3. **`DeleteAlertsFromRules(filename string, alertsToDelete []string)`**
   - Removes specified alerts from Prometheus rules files
   - Preserves YAML structure and formatting
   - Safely handles multiple alerts across different groups

### CLI Tool Features (`cmd/stale-alerts-analyzer/main.go`)

1. **`parseDuration(s string)`**
   - Custom duration parser supporting extended units (d, w, M, y)
   - Falls back to standard Go parsing for compatibility
   - Provides clear error messages for invalid formats

2. **`formatDurationHuman(d time.Duration)`**
   - Converts durations to human-readable format
   - Automatically selects most appropriate unit
   - Examples: `12M`, `90d`, `2w`, `1y`

3. **Rich CLI Output**
   - Color-coded alerts (✓ for active, ⚠ for stale)
   - Detailed timestamps and age calculations
   - Summary statistics with breakdown

## Testing

### Unit Tests

Added comprehensive tests in `internal/alertmanager/hysteresis_test.go`:

- `TestGetAlertNamesFromRules`: Validates alert name extraction
- `TestDeleteAlertsFromRules`: Tests single alert deletion
- `TestDeleteMultipleAlertsFromRules`: Tests bulk deletion
- All tests use temporary files for safe I/O operations
- Proper error handling and resource cleanup

### Test Coverage

- All tests passing with race detection enabled
- Coverage: 34.7% for alertmanager package (new functionality)
- Zero linter issues (golangci-lint)

### CI/CD Status

✅ All CI checks passed:
- Tests (with `-race` flag)
- Linter (golangci-lint v2.8)
- Build (all 4 CLI tools)

## Build Integration

Updated `Makefile` to include `stale-alerts-analyzer` in the standard build pipeline:

```makefile
TOOLS := promql-fmt label-check alert-hysteresis stale-alerts-analyzer
```

Individual build target added:
```bash
make stale-alerts-analyzer
```

## Dependencies

No new external dependencies required. Uses existing:
- `gopkg.in/yaml.v3` (already in use)
- Standard library packages

## Files Changed

- `cmd/stale-alerts-analyzer/main.go` (new)
- `internal/alertmanager/hysteresis.go` (enhanced)
- `internal/alertmanager/hysteresis_test.go` (tests added)
- `Makefile` (updated)
- `.gitignore` (added `coverage.txt`)

## Original Prompts

This work was completed based on the following prompts:

### Prompt 1: Initial Tool Creation

```
Create a new branch and create another CLi tool, to analyse all alerts to see
when they last fired. For any alert that has not fired in X time interval
(default to 12 months), propose deleting that alert. When run with `--fix`,
it should actually delete the stale alert which has not fired
```

### Prompt 2: Enhanced Duration Support

```
Rename the `--threshold` flag to `--timehorizon` and update it to support
multiple units, e.g h for hours, d for days, m for min, M for months, etc
```

## Breaking Changes

None. This is a new tool with no impact on existing functionality.

## Future Enhancements

Potential improvements for future iterations:

1. Support for alert annotations and labels in output
2. Export results to JSON/CSV for further analysis
3. Dry-run mode showing what would be deleted without `--fix`
4. Integration with alertmanager API for runtime alert status
5. Support for multiple rules files in a single run

## Checklist

- [x] Code follows project style guidelines
- [x] All tests passing with race detection
- [x] No linter issues
- [x] Documentation updated (help text, examples)
- [x] Makefile updated for new tool
- [x] Commit messages follow conventional commits format
- [x] Co-authored by attribution included
