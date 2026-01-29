# Agentic Loops - Advanced Autonomous Execution

Comanda's agentic loops enable truly long-running autonomous workflows with state persistence, quality validation, and multi-loop orchestration.

## Quick Start

**Simple stateful loop with resume:**
```bash
comanda process examples/agentic-loop/long-running-resume.yaml
# Press Ctrl-C to interrupt
comanda loop status  # Check saved state
comanda process examples/agentic-loop/long-running-resume.yaml  # Resumes automatically
```

**Multi-loop workflow:**
```bash
comanda process examples/agentic-loop/creator-checker.yaml
```

## Core Features

### 🔄 State Persistence & Resume

Loops can save their state and resume after interruption:

```yaml
agentic-loop:
  config:
    name: my-loop
    stateful: true
    checkpoint_interval: 5  # Save every 5 iterations
    timeout_seconds: 0      # No timeout
```

- State saved to: `~/.comanda/loop-states/{loop-name}.json`
- Automatic backup rotation (keeps last 3 versions)
- Resume by re-running the same workflow file

### ✅ Quality Gates

Run automated validation after each iteration:

```yaml
quality_gates:
  - name: typecheck
    command: "npm run typecheck"
    on_fail: retry
    timeout: 60
    retry:
      max_attempts: 3
      backoff_type: exponential
      initial_delay: 5

  - name: tests
    command: "npm test"
    on_fail: abort

  - name: security
    type: security  # Built-in security scanner
    on_fail: skip
```

**Built-in gate types:**
- `syntax` - Auto-detect syntax errors (Python, JS, Go, TypeScript, etc.)
- `security` - Scan for secrets, SQL injection, security issues
- `test` - Run custom test commands

**Failure actions:**
- `retry` - Retry with exponential/linear backoff
- `abort` - Stop loop and save state
- `skip` - Log warning and continue

### 🔗 Multi-Loop Orchestration

Define multiple loops with dependencies:

```yaml
loops:
  data-collector:
    name: data-collector
    output_state: $RAW_DATA
    # ... config ...

  data-analyzer:
    name: data-analyzer
    depends_on: [data-collector]
    input_state: $RAW_DATA
    # ... config ...
```

**Features:**
- Automatic execution ordering (topological sort)
- Variable passing between loops
- Cycle detection with clear error messages
- Independent loop state management

### 🎯 Creator/Checker Pattern

Implement validation workflows with automatic retries:

```yaml
workflow:
  creator:
    type: loop
    loop: feature-creator
    role: creator

  checker:
    type: loop
    loop: code-checker
    role: checker
    validates: creator
    on_fail: rerun_creator
```

**How it works:**
1. Creator loop implements feature
2. Checker loop validates (looks for "PASS" or "FAIL" in output)
3. If validation fails, creator automatically reruns (up to 3 times)
4. Continues until validation passes

### 📋 Loop Management CLI

```bash
# List all loops
comanda loop status

# Detailed status
comanda loop status my-loop

# Resume a paused/failed loop
comanda loop resume my-loop

# Cancel a loop
comanda loop cancel my-loop

# Clean up completed loops
comanda loop clean
```

## Examples

All examples are in this directory:

### Basic Features
- **`simple-refinement.yaml`** - Basic agentic loop
- **`long-running-resume.yaml`** - State persistence & resume
- **`quality-gate-retry.yaml`** - Quality gates with retry
- **`quality-gate-abort.yaml`** - Quality gates with abort
- **`code-quality-loop.yaml`** - Real-world code improvement loop

### Multi-Loop Orchestration
- **`simple-multi-loop.yaml`** - Basic sequential execution
- **`dependency-graph.yaml`** - Complex dependency chain
- **`creator-checker.yaml`** - Creator/checker pattern
- **`cycle-detection.yaml`** - Cycle detection demo

## Documentation

- **[README-STATE-AND-QUALITY.md](README-STATE-AND-QUALITY.md)** - Detailed guide to state persistence and quality gates
- **[README-ORCHESTRATION.md](README-ORCHESTRATION.md)** - Complete multi-loop orchestration guide

## Configuration Reference

### Basic Loop Config

```yaml
agentic-loop:
  config:
    name: my-loop
    max_iterations: 100
    timeout_seconds: 0
    exit_condition: llm_decides
    context_window: 5

    # State persistence
    stateful: true
    checkpoint_interval: 5

    # Quality gates
    quality_gates:
      - name: validation
        command: "make test"
        on_fail: abort

  steps:
    - task:
        input: NA
        model: claude-code
        action: "Your task here"
        output: STDOUT
```

### Multi-Loop Config

```yaml
loops:
  loop-a:
    name: loop-a
    output_state: $RESULT_A
    # ... steps ...

  loop-b:
    name: loop-b
    depends_on: [loop-a]
    input_state: $RESULT_A
    # ... steps ...

execute_loops:
  - loop-a
  - loop-b
```

### Workflow Config

```yaml
loops:
  creator: { ... }
  checker: { ... }

workflow:
  create:
    type: loop
    loop: creator
    role: creator

  validate:
    type: loop
    loop: checker
    role: checker
    validates: create
    on_fail: rerun_creator
```

## Best Practices

### 1. Use Descriptive Loop Names
```yaml
# Good
name: feature-implementation-auth-service

# Bad
name: loop1
```

### 2. Set Appropriate Checkpoints
- Fast iterations (< 1 min): `checkpoint_interval: 10`
- Medium iterations (1-5 min): `checkpoint_interval: 5` (default)
- Long iterations (> 5 min): `checkpoint_interval: 1`

### 3. Use Quality Gates Strategically
```yaml
quality_gates:
  # Critical: abort on failure
  - name: syntax
    type: syntax
    on_fail: abort

  # Important: retry a few times
  - name: tests
    command: "npm test"
    on_fail: retry
    retry:
      max_attempts: 3

  # Nice-to-have: skip on failure
  - name: linting
    command: "npm run lint"
    on_fail: skip
```

### 4. Design Clear Loop Boundaries
```yaml
# Good: Focused, single-responsibility loops
loops:
  fetch-data: { ... }
  analyze-data: { ... }
  create-report: { ... }

# Bad: Monolithic loop
loops:
  do-everything: { ... }
```

### 5. Use Meaningful Variable Names
```yaml
# Good
output_state: $USER_DATA
output_state: $ANALYSIS_RESULTS

# Bad
output_state: $DATA
output_state: $RESULT
```

## Common Patterns

### Iterative Improvement
```yaml
agentic-loop:
  config:
    name: code-refinement
    stateful: true
    max_iterations: 10
    quality_gates:
      - name: tests
        command: "pytest"
        on_fail: retry
```

### Data Pipeline
```yaml
loops:
  extract:
    output_state: $RAW_DATA
  transform:
    depends_on: [extract]
    input_state: $RAW_DATA
    output_state: $CLEAN_DATA
  load:
    depends_on: [transform]
    input_state: $CLEAN_DATA
```

### Validation Loop
```yaml
workflow:
  implement:
    loop: implementation
    role: creator
  review:
    loop: code-review
    role: checker
    validates: implement
    on_fail: rerun_creator
```

## Troubleshooting

### Loop Won't Resume
**Problem:** "no saved state found for loop 'my-loop'"

**Solutions:**
1. Check loop name matches: `comanda loop status`
2. Verify state file exists: `ls ~/.comanda/loop-states/`
3. Ensure loop has `stateful: true` and `name` set

### Workflow Modified Error
**Problem:** "workflow file has been modified since loop started"

**Solution:** This is a safety check. Either:
- Start fresh (deletes old state automatically)
- Or manually delete: `comanda loop cancel my-loop`

### Quality Gate Timeout
**Problem:** Gate times out before completing

**Solution:** Increase timeout:
```yaml
quality_gates:
  - name: slow-test
    command: "npm test"
    timeout: 600  # 10 minutes
```

### Cycle Detected
**Problem:** "dependency cycle detected: loop-a -> loop-b -> loop-a"

**Solution:**
1. Draw dependency graph on paper
2. Remove circular dependencies
3. Consider if loops should be merged

## Performance Tips

### State File Location
State files are stored in `~/.comanda/loop-states/`
- Each loop has its own state file
- Automatic backup rotation (`.json.1`, `.json.2`, `.json.3`)
- Clean up periodically: `comanda loop clean`

### Memory Considerations
Variables stored in memory during orchestration:
- Large outputs = large memory usage
- For bulk data, use file paths instead of variables
- Use variables for metadata and coordination

## Advanced Topics

For detailed information on specific features:

- **State Persistence & Quality Gates:** [README-STATE-AND-QUALITY.md](README-STATE-AND-QUALITY.md)
- **Multi-Loop Orchestration:** [README-ORCHESTRATION.md](README-ORCHESTRATION.md)

## Feedback & Support

Report issues at: https://github.com/kris-hansen/comanda/issues
