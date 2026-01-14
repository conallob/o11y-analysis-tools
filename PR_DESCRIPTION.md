# Fix PromQL Multiline Formatting and Add Comprehensive Validation

## Summary

This PR implements proper multiline formatting for PromQL expressions with aggregation optimization and adds comprehensive validation for Prometheus naming best practices. The implementation follows official Prometheus documentation for metric naming, label naming, and recording rule conventions.

## Major Changes

### 1. Multiline Formatting with Indented Operators
- Each operand on its own line(s)
- Binary operators indented by 2 spaces
- Proper handling of nested expressions and parentheses
- **Example:**
  ```yaml
  expr: |
    sum (
      rate(http_requests_total{job="api",status=~"5.."}[5m])
    )
      / on (instance)
    sum by (instance) (
      rate(http_requests_total{job="api"}[5m])
    )
  ```

### 2. Aggregation Clause Optimization
- Removes redundant aggregation clauses from left operand
- Preserves `without` clauses on both sides (required by PromQL semantics)
- Exception: `without` keyword needs labels on both operands
- **Example:** `sum(...) by (instance) / sum(...) by (instance)` → `sum(...) / sum(...) by (instance)`

### 3. Explicit Vector Matching with `on()` Clauses
- Automatically generates `on()` clauses for explicit vector matching
- Extracts labels from aggregation clauses
- Improves query clarity and prevents ambiguity
- **Example:** `/ on (instance)` instead of implicit matching

### 4. Synthetic Metrics Validation
- Validates that `up` metric always includes job label selector
- Prevents matching multiple jobs erroneously
- Handles all contexts: standalone, aggregations, range selectors

### 5. Variable/Metric Naming Validation
- Enforces `snake_case` convention (lowercase with underscores)
- Prohibits camelCase and PascalCase
- Prevents metric type suffixes (_gauge, _counter, _histogram, _summary)
- Validates character set: `[a-zA-Z_:][a-zA-Z0-9_:]*`

### 6. Label Naming Validation
- Validates label names match `[a-zA-Z_][a-zA-Z0-9_]*`
- Prohibits leading underscores (reserved for internal use)
- Detects double leading underscores (reserved for Prometheus)
- Warns about overly generic label names (e.g., "type")

### 7. Recording Rule Naming Validation
- Enforces `level:metric:operations` format
- Validates each component (level, metric, operations)
- Recommends stripping `_total` suffix when using `rate()`/`irate()`
- Prohibits ambiguous suffixes: `:value` and `:avg` (without time window)
- **Traceability:** Level component provides breadcrumb through aggregation levels

## Original Prompts

### Prompt 1: Multiline Formatting
```
The documented example for promql-fmt does not format the multiline PromQL
expression correctly. The correct format should lay out each operand expression
on it's own line, with the operator in between both operands being indented by
2 spaces...
```

### Prompt 2: Test Coverage
```
Also make sure @cmd/promql-fmt performs the formatting the same way, including
in it's test coverage
```

### Prompt 3: Aggregation Optimization
```
In an expression like `sum by (instance) (...) / sum by (instance) (...)`,
common labels in both operands only need to be specified in the final operand.
The only time when both operands need to be explicit about their labels is when
using the `without` keyword...
```

### Prompt 4: Label Selector Optimization
```
Let's implement both checks, one for the aggregation clauses and a second check
for reducing duplication in the label selectors
```

### Prompt 5: Vector Matching Spacing
```
There is a small nit in your example. Instead of `/ on(instance)`, it should be
`/ on (instance)`
```

### Prompt 6: Synthetic Metrics Validation
```
Check for use of the synthetic 'up' variables, without specifying a job label
selector. The synthetic up variable should always be qualified with a job label
selector. Otherwise it will match erroneously in any system with more than one
value for job
```

### Prompt 7: Comprehensive Naming Validation
```
Add checks for variable naming per https://prometheus.io/docs/practices/naming/
and rule naming per https://prometheus.io/docs/practices/rules/, without
specifying careful consideration to preserving the variable name throughout
rules at different levels of aggregation, providing a breadcrumb to trace
through the system
```

### Prompt 8: Ambiguous Suffixes
```
Other rule suffixes to avoid are :value and :avg
```

## Implementation Details

### Files Modified
- `pkg/formatting/promql.go` - Core formatting and validation logic
- `pkg/formatting/promql_test.go` - Comprehensive test coverage
- `README.md` - Updated example to show correct formatting

### New Functions
- `formatPromQLMultiline()` - Multiline formatting with optimization
- `splitByBinaryOperator()` - Splits expressions respecting parentheses
- `formatOperand()` - Formats operands with aggregation handling
- `checkSyntheticMetrics()` - Validates synthetic metric usage
- `checkVariableNaming()` - Validates metric/variable naming
- `checkLabelNaming()` - Validates label naming conventions
- `checkRecordingRuleNaming()` - Validates recording rule format

### Test Coverage
- 6 test cases for multiline formatting
- 9 test cases for synthetic metrics validation
- 9 test cases for variable naming validation
- 10 test cases for label naming validation
- 17 test cases for recording rule naming validation

All tests pass with race detection enabled (`go test -race`).

## Validation Examples

### Variable Naming
❌ `httpRequestsTotal` → should be `http_requests_total`
❌ `memory_usage_gauge` → should be `memory_usage`
✅ `http_requests_total`

### Label Naming
❌ `{_internal="true"}` → leading underscore reserved
❌ `{__name__="test"}` → double underscore reserved for Prometheus
❌ `{type="api"}` → too generic
✅ `{job="api",instance="localhost"}`

### Recording Rule Naming
❌ `job:httpRequestsTotal:rate5m` → metric should be snake_case
❌ `job:http_requests_total:rate5m` → should strip _total: `job:http_requests:rate5m`
❌ `job:cpu_usage:value` → :value suffix is ambiguous
❌ `job:cpu_usage:avg` → should specify time window: `avg5m`
✅ `job:http_requests:rate5m`
✅ `job_instance:http_requests:rate5m` (aggregated further)

## Traceability Example

Recording rules preserve metric names through aggregation levels:
```
Original metric:      http_requests_total{job="api", instance="localhost"}
Aggregated by job:    job:http_requests:rate5m
Further aggregation:  job_instance:http_requests:rate5m
Cluster level:        cluster:http_requests:rate5m
```

The level component changes to reflect which labels remain, providing a clear breadcrumb trail through the system.

## References

### Official Prometheus Documentation
- [Metric and label naming | Prometheus](https://prometheus.io/docs/practices/naming/)
- [Recording rules | Prometheus](https://prometheus.io/docs/practices/rules/)
- [Data model | Prometheus](https://prometheus.io/docs/concepts/data_model/)

### Community Best Practices
- [Prometheus metric naming recommendations - Chronosphere](https://docs.chronosphere.io/ingest/metrics-traces/collector/mappings/prometheus/prometheus-recommendations)
- [Rule Naming Conventions - PromLabs](https://training.promlabs.com/training/recording-rules/recording-rules-overview/rule-naming-conventions/)

## Commits

1. `b8a228e` - docs: fix promql-fmt multiline formatting example
2. `a1a664a` - feat(promql-fmt): implement proper multiline formatting with indented operators
3. `fd1a40f` - feat(promql-fmt): optimize redundant aggregation clauses in binary operations
4. `d61a0ee` - feat(promql-fmt): add explicit on() clauses for vector matching clarity
5. `eb0c950` - fix(promql-fmt): add space between on and parenthesis in vector matching
6. `6e475e6` - feat(promql-fmt): add validation for synthetic 'up' metric job label
7. `37da6be` - feat(promql-fmt): add variable, label, and recording rule naming validations
8. `e298b82` - feat(promql-fmt): add validation for ambiguous recording rule suffixes
9. `7eb7658` - fix(promql-fmt): resolve golangci-lint errors

## Testing

```bash
# Run all tests with race detection
go test -v -race ./pkg/formatting

# Run linting
golangci-lint run

# Build all binaries
go build -o bin/ ./cmd/promql-fmt
go build -o bin/ ./cmd/label-check
go build -o bin/ ./cmd/alert-hysteresis
```

All tests pass, no linting errors, and all binaries build successfully.
