# Agentic Loops - Phase 1: Long-Running Autonomous Execution

Phase 1 adds state persistence, quality gates, and unlimited runtime to Comanda's agentic loops, enabling truly long-running (hours/days) autonomous execution.

## New Features

### 1. State Persistence & Resume

Loops can now save their state to disk and resume after interruption:

```yaml
agentic-loop:
  config:
    name: my-loop              # Required for stateful loops
    stateful: true             # Enable state persistence
    checkpoint_interval: 5     # Save every 5 iterations
    max_iterations: 100
    timeout_seconds: 0         # No timeout
```

**State is saved to:** `~/.comanda/loop-states/{loop-name}.json`

**Resume a loop:**
```bash
# The loop automatically resumes if you re-run the workflow
comanda process my-workflow.yaml

# Or check status first
comanda loop status my-loop
```

### 2. Quality Gates

Run automated checks after each iteration with retry logic:

```yaml
agentic-loop:
  config:
    quality_gates:
      - name: typecheck
        command: "npm run typecheck"
        on_fail: retry
        timeout: 60
        retry:
          max_attempts: 3
          backoff_type: exponential  # or linear
          initial_delay: 5

      - name: tests
        command: "npm test"
        on_fail: abort
        timeout: 300

      - name: security
        type: security  # Built-in security scanner
        on_fail: skip
```

**Built-in Gate Types:**
- `syntax` - Checks for syntax errors (Python, JS, Go, etc.)
- `security` - Scans for hardcoded secrets, security issues
- `test` - Runs custom test commands

**On Failure Actions:**
- `retry` - Retry with backoff (default: 3 attempts)
- `abort` - Stop loop and save state as "failed"
- `skip` - Log warning and continue

### 3. No Timeout by Default

Loops can now run indefinitely:

```yaml
agentic-loop:
  config:
    timeout_seconds: 0  # No timeout (default)
    max_iterations: 1000
```

Control is via `max_iterations` and exit conditions instead of time limits.

### 4. Loop Management CLI

New `comanda loop` command for managing loop state:

```bash
# List all loops
comanda loop status

# Detailed status for a specific loop
comanda loop status my-loop

# Resume a paused/failed loop
comanda loop resume my-loop

# Cancel a loop
comanda loop cancel my-loop

# Clean up completed/failed loops
comanda loop clean
```

## Examples

### Example 1: Long-Running with Resume

See: `long-running-resume.yaml`

Demonstrates a loop that:
- Runs for 100 iterations
- Saves checkpoints every 5 iterations
- Can be interrupted and resumed

**Test resume:**
```bash
# Start the loop
comanda process examples/agentic-loop/long-running-resume.yaml

# Press Ctrl-C after a few iterations

# Check status
comanda loop status long-test

# Resume (re-run the same command)
comanda process examples/agentic-loop/long-running-resume.yaml
```

### Example 2: Quality Gates with Retry

See: `quality-gate-retry.yaml`

Demonstrates:
- Flaky test that fails 50% of the time
- Exponential backoff retry (1s, 2s, 4s, 8s, 16s)
- Multiple gates with different failure policies

```bash
comanda process examples/agentic-loop/quality-gate-retry.yaml
```

You'll see retry attempts logged with increasing delays.

### Example 3: Quality Gate Abort

See: `quality-gate-abort.yaml`

Demonstrates:
- Gate that always fails
- Loop aborts after first iteration
- State saved as "failed"

```bash
comanda process examples/agentic-loop/quality-gate-abort.yaml

# Check the failed state
comanda loop status abort-test
```

### Example 4: Code Quality Loop

See: `code-quality-loop.yaml`

Real-world example:
- Multiple quality gates (syntax, security)
- Agentic tool access for code modification
- Iterative improvement until quality is acceptable

```bash
comanda process examples/agentic-loop/code-quality-loop.yaml
```

## State File Structure

State files are stored in `~/.comanda/loop-states/{loop-name}.json`:

```json
{
  "loop_name": "my-loop",
  "iteration": 15,
  "max_iterations": 100,
  "start_time": "2026-01-28T10:00:00Z",
  "last_update_time": "2026-01-28T10:15:00Z",
  "previous_output": "...",
  "history": [...],
  "variables": {...},
  "status": "running",
  "quality_gate_results": [...]
}
```

**Automatic backup rotation:** Last 3 states saved as `.json.1`, `.json.2`, `.json.3`

## Configuration Reference

### AgenticLoopConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | - | Loop name (required for stateful) |
| `stateful` | bool | false | Enable state persistence |
| `checkpoint_interval` | int | 5 | Save state every N iterations |
| `max_iterations` | int | 10 | Maximum iterations |
| `timeout_seconds` | int | 0 | Timeout (0 = no timeout) |
| `quality_gates` | []QualityGateConfig | - | Quality gates to run |

### QualityGateConfig

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✓ | Gate name |
| `command` | string | - | Shell command to execute |
| `type` | string | - | Built-in type: syntax, security, test |
| `on_fail` | string | ✓ | Action: retry, skip, abort |
| `timeout` | int | - | Timeout in seconds |
| `retry` | RetryConfig | - | Retry configuration |

### RetryConfig

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | 3 | Maximum retry attempts |
| `backoff_type` | string | linear | linear or exponential |
| `initial_delay` | int | 1 | Initial delay in seconds |

## Best Practices

### 1. Use Descriptive Loop Names

```yaml
# Good
name: feature-implementation-auth-service

# Bad
name: loop1
```

### 2. Set Appropriate Checkpoint Intervals

- **Fast iterations (< 1 min):** `checkpoint_interval: 10`
- **Medium iterations (1-5 min):** `checkpoint_interval: 5` (default)
- **Long iterations (> 5 min):** `checkpoint_interval: 1`

### 3. Use Quality Gates Strategically

```yaml
quality_gates:
  # Critical checks: abort on failure
  - name: syntax
    type: syntax
    on_fail: abort

  # Important checks: retry a few times
  - name: tests
    command: "npm test"
    on_fail: retry
    retry:
      max_attempts: 3

  # Nice-to-have checks: skip on failure
  - name: linting
    command: "npm run lint"
    on_fail: skip
```

### 4. Monitor Loop Progress

```bash
# Watch loop progress in real-time
watch -n 5 'comanda loop status my-loop'
```

### 5. Clean Up Completed Loops

```bash
# Periodically clean up old states
comanda loop clean
```

## Troubleshooting

### Loop Won't Resume

**Problem:** "no saved state found for loop 'my-loop'"

**Solutions:**
1. Check loop name matches exactly: `comanda loop status`
2. Verify state file exists: `ls ~/.comanda/loop-states/`
3. Check file permissions: `ls -l ~/.comanda/loop-states/`

### Workflow Modified Error

**Problem:** "workflow file has been modified since loop started"

**Solution:** This is a safety check. The workflow file changed. Either:
1. Start a fresh loop (will create new state)
2. Manually delete old state: `comanda loop cancel my-loop`

### Quality Gate Timeout

**Problem:** Gate times out before completing

**Solution:** Increase gate timeout:
```yaml
quality_gates:
  - name: slow-test
    command: "npm test"
    timeout: 600  # 10 minutes
```

### State File Corrupted

**Problem:** "failed to unmarshal state (file may be corrupted)"

**Solution:** Restore from backup:
```bash
cd ~/.comanda/loop-states
cp my-loop.json.1 my-loop.json  # Restore from backup 1
```

## What's Next: Phase 2

Phase 2 will add multi-loop orchestration with creator/checker patterns:

```yaml
loops:
  creator:
    name: feature-creator
    # ... implements features

  checker:
    name: code-checker
    depends_on: [creator]
    # ... validates creator's work

workflow:
  creator:
    type: loop
    loop: feature-creator
  checker:
    type: loop
    loop: code-checker
    validates: creator
    on_fail: rerun_creator
```

See the main plan document for full Phase 2 details.

## Implementation Details

**New Files:**
- `utils/processor/loop_state.go` - State persistence
- `utils/processor/quality_gates.go` - Quality gate framework
- `cmd/loop.go` - CLI commands

**Modified Files:**
- `utils/processor/agentic_loop.go` - State integration
- `utils/processor/types.go` - New config types
- `utils/config/env.go` - Loop state directory

**State Directory:** `~/.comanda/loop-states/`

## Feedback & Issues

Phase 1 implementation completed January 2026.

Report issues at: https://github.com/kris-hansen/comanda/issues
