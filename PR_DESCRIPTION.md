# Pull Request: Add PromQL test automation and E2E alertmanager testing tools

**Branch:** `claude/promql-test-tools-BNb2s` → `main`

## Summary

This PR introduces two new CLI tools for enhanced Prometheus/Alertmanager workflow automation:

1. **autogen-promql-tests**: Automatically identifies alerts and recording rules without test coverage and generates comprehensive unit tests
2. **e2e-alertmanager-test**: End-to-end integration testing that renders complete notification bodies across multiple platforms for UX development

## Original Prompts

### Initial Request
```
Create a new branch for 2 additional CLI commands:
* autogen-promql-tests, which identifies rules and alerts in a named file which have
  no corresponding test coverage. When run with --fix, it will generate a test block
  for the rule/alert in the correct test file that includes:
  - A true positive test case (alert should fire)
  - A false positive/true negative test case (alert should NOT fire)
  - A hysteresis threshold check for the 'for' duration
  - A placeholder for edge cases to be added manually

* e2e-alertmanager-test: E2E integration test that evaluates all alert_rule_test cases,
  feeds firing alerts through alertmanager binary, shows how alertmanager rules match
  alert definitions and labels. Format output as RFC 2076 compliant SMTP email headers
  and body. May need boilerplate alertmanager config to mimic live alertmanager.
```

### Enhancement Request
```
Expand e2e-alertmanager-test to also render the full body of a notification,
so it can be used to develop and diff UX changes
```

## Changes

### New CLI Tools

#### 1. autogen-promql-tests (\`cmd/autogen-promql-tests/main.go\`)

**Purpose**: Identifies Prometheus alerts and recording rules without test coverage and auto-generates comprehensive test cases.

**Key Features**:
- Auto-discovers test files (e.g., \`alerts.yml\` → \`alerts_test.yml\`)
- Analyzes coverage to find untested rules/alerts
- Generates 4 test cases per alert:
  * True positive (alert should fire)
  * False positive/true negative (alert should NOT fire)
  * Hysteresis testing for \`for\` duration
  * Edge case placeholder for manual additions
- \`--fix\` mode writes tests directly to files

**Usage**:
```bash
# Identify untested alerts
autogen-promql-tests --rules=alerts.yml

# Generate tests automatically
autogen-promql-tests --rules=alerts.yml --fix
```

#### 2. e2e-alertmanager-test (\`cmd/e2e-alertmanager-test/main.go\`)

**Purpose**: End-to-end integration testing with full notification rendering for UX development and visual diffing.

**Key Features**:
- Sends alerts to Alertmanager API
- Renders complete notification bodies in multiple formats:
  * **HTML Email**: Modern responsive design with severity-based color theming
  * **Slack**: Attachment format with proper color mapping
  * **Webhook/JSON**: Full alert payload
- RFC 2076 compliant SMTP email headers
- Multiple output modes for different use cases
- Shows alertmanager routing information

**Usage**:
```bash
# HTML email preview
e2e-alertmanager-test --tests=./alerts_test.yml --output=email-html --full

# All notification formats for UX diffing
e2e-alertmanager-test --tests=./alerts_test.yml --output=full > notifications.txt

# Slack preview
e2e-alertmanager-test --tests=./alerts_test.yml --output=slack

# Test with live alertmanager
e2e-alertmanager-test --tests=./alerts_test.yml \\
                       --alertmanager-url=http://localhost:9093 \\
                       --alertmanager-config=./alertmanager.yml
```

**Output Formats**:
- \`email\` - Plain text email with RFC 2076 headers
- \`email-html\` - Styled HTML email rendering
- \`slack\` - Slack message attachment format
- \`json\` - JSON output
- \`full\` - All notification formats combined

### Implementation Highlights

#### autogen-promql-tests
- YAML parsing with \`gopkg.in/yaml.v3\`
- Intelligent test file discovery
- Template-based test generation with realistic examples
- Support for both recording rules and alert rules

#### e2e-alertmanager-test
- **NotificationOutput** struct for multi-format support
- **HTML Email Rendering**:
  - Severity color mapping (critical=#d32f2f, warning=#f57c00, info=#1976d2)
  - Modern CSS with responsive design
  - Multipart MIME support
- **Slack Rendering**:
  - Color mapping (critical=danger, warning=warning, info=good)
  - Structured field attachments
- **Alertmanager Integration**:
  - POST to \`/api/v2/alerts\` endpoint
  - Config parsing for routing information
  - Proper error handling with defer patterns

### Build Integration

Updated \`Makefile\` to include both new tools:
```makefile
TOOLS := promql-fmt label-check alert-hysteresis autogen-promql-tests e2e-alertmanager-test
```

Individual build targets added for each tool.

## Test Plan

- [x] All existing tests pass with race detection
- [x] golangci-lint clean (0 issues)
- [x] All 5 CLI tools build successfully on linux/amd64
- [x] autogen-promql-tests identifies untested alerts correctly
- [x] autogen-promql-tests generates valid test YAML with --fix
- [x] e2e-alertmanager-test sends alerts to Alertmanager API
- [x] e2e-alertmanager-test renders RFC 2076 compliant email output
- [x] e2e-alertmanager-test renders HTML email with proper styling
- [x] e2e-alertmanager-test renders Slack format with color mapping
- [x] Multiple output formats work correctly (email, email-html, slack, json, full)

## Files Changed

- \`Makefile\` - Added new tools to build targets
- \`cmd/autogen-promql-tests/main.go\` - New CLI tool (452 lines)
- \`cmd/e2e-alertmanager-test/main.go\` - New CLI tool (657 lines)

**Total**: 3 files changed, 1,118 insertions(+), 4 deletions(-)

## Benefits

1. **Development Velocity**: \`autogen-promql-tests\` eliminates manual test writing for basic coverage
2. **UX Iteration**: \`e2e-alertmanager-test\` enables rapid iteration on notification design across platforms
3. **Visual Diffing**: Full notification rendering makes it easy to compare before/after changes
4. **Platform Coverage**: Single tool tests email, Slack, and webhook notifications
5. **Quality Assurance**: Comprehensive test generation ensures alerts are properly validated

## Breaking Changes

None - these are new tools with no impact on existing functionality.

## Related Issues

N/A - New feature implementation.

---

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
