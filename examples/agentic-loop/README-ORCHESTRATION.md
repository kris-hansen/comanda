# Agentic Loops - Multi-Loop Orchestration
This guide covers multi-loop orchestration features including dependency management, variable passing, and creator/checker patterns for complex autonomous workflows.

## Features
### 1. Named Loops with Dependencies
Define multiple loops that depend on each other:

```yaml
loops:
  data-collector:
    name: data-collector
    # ... loop config ...
    output_state: $RAW_DATA  # Export variable

  data-analyzer:
    name: data-analyzer
    depends_on: [data-collector]  # Wait for collector
    input_state: $RAW_DATA        # Read collector's output
    # ... loop config ...
```
**Key capabilities:**

- Loops execute in dependency order (topological sort)
- Data flows between loops via variables
- Automatic cycle detection with clear error messages

### 2. Variable Passing
Loops can pass data to dependent loops:

```yaml
loops:
  producer:
    output_state: $MY_DATA  # Export to variable
    # ...

  consumer:
    depends_on: [producer]
    input_state: $MY_DATA   # Read from variable
    # ...
```
**Variable flow:**

- `output_state` - Exports loop result to a variable
- `input_state` - Reads variable as loop input
- `depends_on` - Waits for dependent loops (can also use their results directly)

### 3. Creator/Checker Pattern
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
    on_fail: rerun_creator  # Auto-rerun on validation failure
```
**Validation flow:**

1. Creator loop implements feature
2. Checker loop validates implementation
3. If checker outputs contains "PASS" → workflow succeeds
4. If checker outputs contains "FAIL" → rerun creator (up to 3 times)
5. Repeat until validation passes or max attempts reached

**on_fail options:**

- `rerun_creator` - Automatically rerun the creator loop
- `abort` - Stop workflow immediately
- `manual` - Return for manual review

### 4. Execution Models
Two ways to orchestrate loops:

Simple Execution (execute_loops)

```yaml
loops:
  task-a: { ... }
  task-b:
    depends_on: [task-a]
    # ...

execute_loops:
  - task-a
  - task-b
```
Executes loops in dependency order (topological sort).

Workflow Execution (workflow)

```yaml
workflow:
  node1:
    type: loop
    loop: my-loop
    role: creator

  node2:
    type: loop
    loop: validator
    role: checker
    validates: node1
    on_fail: rerun_creator
```
Advanced execution with roles and validation relationships.

## Examples
### Example 1: Creator/Checker Pattern
See: `creator-checker.yaml`

Demonstrates:

- Creator loop implements a feature (email validator)
- Checker loop validates implementation
- Automatic rerun on validation failure (up to 3 attempts)
- Workflow completes when validation passes

```bash
comanda process examples/agentic-loop/creator-checker.yaml
```
**Expected flow:**

1. Creator implements basic email validator
2. Checker reviews → might output "FAIL - missing edge cases"
3. Creator reruns, improves implementation
4. Checker reviews → outputs "PASS - code is correct"
5. Workflow completes successfully

### Example 2: Dependency Graph
See: `dependency-graph.yaml`

Demonstrates:

- Three loops with sequential dependencies
- Data flows through the chain: collector → analyzer → reporter
- Variables passed between loops
- Topological sort execution

```bash
comanda process examples/agentic-loop/dependency-graph.yaml
```
**Execution order:**

1. `data-collector` - Generates sample data, exports to `$RAW_DATA`
2. `data-analyzer` - Reads `$RAW_DATA`, analyzes, exports to `$ANALYSIS`
3. `report-generator` - Reads `$ANALYSIS`, creates final report

### Example 3: Simple Multi-Loop
See: `simple-multi-loop.yaml`

Demonstrates:

- Basic sequential execution
- Variable passing between loops
- Simple dependency chain (A → B → C)

```bash
comanda process examples/agentic-loop/simple-multi-loop.yaml
```
### Example 4: Cycle Detection
See: `cycle-detection.yaml`

Demonstrates:

- Automatic cycle detection
- Clear error reporting
- Safety mechanism to prevent infinite loops

```bash
comanda process examples/agentic-loop/cycle-detection.yaml
```
**Expected output:**

```
Error: dependency cycle detected: loop-a -> loop-c -> loop-b -> loop-a
```
## Configuration Reference
### AgenticLoopConfig (additions)
| Field | Type | Description |
|-------|------|-------------|
| `depends_on` | []string | Loops that must complete before this one |
| `input_state` | string | Variable to read as input (e.g., `$MY_VAR`) |
| `output_state` | string | Variable to export result to (e.g., `$MY_VAR`) |



### DSLConfig (additions)
| Field | Type | Description |
|-------|------|-------------|
| `loops` | map[string]AgenticLoopConfig | Named loops for orchestration |
| `execute_loops` | []string | Simple execution order |
| `workflow` | map[string]WorkflowNode | Complex workflow definition |



### WorkflowNode
| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Node type (always "loop" for now) |
| `loop` | string | Loop name to execute |
| `role` | string | Node role: creator, checker, finalizer |
| `validates` | string | Loop name this node validates |
| `on_fail` | string | Action on validation failure: rerun_creator, abort, manual |



## How It Works
### Dependency Graph Execution
1. **Graph Construction**
    - Parse `depends_on` relationships
    - Build directed acyclic graph (DAG)
    - Validate no cycles exist
2. **Topological Sort** (Kahn's Algorithm)
    - Find loops with no dependencies (in-degree = 0)
    - Execute them first
    - Remove from graph, update dependents
    - Repeat until all loops executed
3. **Execution Order**data-collector (no deps) → data-analyzer (depends on collector) → report-generator (depends on analyzer)

### Workflow Execution
1. **Workflow Parsing**
    - Parse workflow nodes with roles
    - Build execution order from `validates` relationships
    - Identify creator/checker pairs
2. **Creator/Checker Logic**while attempts < max_attempts:
3.   execute checker loop
4.   if "PASS" in output:
5.     break
6.   if "FAIL" in output and on_fail == "rerun_creator":
7.     rerun creator loop
8.     update checker input
9.   else:
10.     abort
11. **Maximum Attempts**
    - Default: 3 attempts
    - Prevents infinite rerun loops
    - Configurable per workflow node

### Variable System
Variables are stored in the processor's variable map:

```go
p.variables["$RAW_DATA"] = collectorOutput
p.variables["$ANALYSIS"] = analyzerOutput
```
**Reading variables:**

- `input_state: $RAW_DATA` → reads from variable map
- If variable doesn't exist → error

**Writing variables:**

- `output_state: $ANALYSIS` → writes loop result to variable map
- Overwrites existing value if present

## Best Practices
### 1. Design Loop Boundaries
```yaml
# Good: Clear, focused loops
loops:
  fetch-data:    # Single responsibility
  analyze-data:  # Single responsibility
  create-report: # Single responsibility

# Bad: Monolithic loop
loops:
  do-everything:  # Too broad
```
### 2. Use Meaningful Variable Names
```yaml
# Good
output_state: $USER_DATA
output_state: $ANALYSIS_RESULTS
output_state: $VALIDATED_CODE

# Bad
output_state: $OUTPUT
output_state: $RESULT
output_state: $DATA
```
### 3. Limit Creator Reruns
Default max attempts (3) is usually sufficient:

- Attempt 1: Basic implementation
- Attempt 2: Fix major issues
- Attempt 3: Polish and edge cases

If validation fails after 3 attempts, there may be a fundamental issue requiring manual intervention.

### 4. Use Explicit Dependencies
```yaml
# Good: Explicit dependencies
loops:
  loop-b:
    depends_on: [loop-a]
    input_state: $LOOP_A_OUTPUT

# Bad: Implicit dependencies (hard to debug)
loops:
  loop-b:
    # Assumes loop-a runs first (not guaranteed!)
    input_state: $LOOP_A_OUTPUT
```
### 5. Validate Checker Output
Checkers should output clear PASS/FAIL signals:

```yaml
# Good checker output
PASS - All validation checks passed
FAIL - Missing edge case handling for empty input

# Bad checker output (ambiguous)
Looks good
Some issues found
```
## Troubleshooting
### Cycle Detected Error
**Problem:** "dependency cycle detected: loop-a -> loop-b -> loop-a"

**Solution:**

1. Review `depends_on` relationships
2. Draw dependency graph on paper
3. Remove circular dependencies
4. Consider if loops should be merged

### Variable Not Found
**Problem:** "input variable '$MY_VAR' not found"

**Solution:**

1. Check that dependent loop has `output_state: $MY_VAR`
2. Verify dependent loop completed successfully
3. Check for typos in variable names (case-sensitive)

### Checker Always Fails
**Problem:** Creator/checker loop hits max attempts

**Solutions:**

1. **Check checker logic:** Is it too strict?
2. **Review creator output:** Is it producing valid results?
3. **Add debugging:**# Creator
4. steps:
5.   - implement:
6.       action: |
7.         # ... existing action ...
8. 
9.         # Debug: Show what was produced
10.         Debug output: \[show key parts]
11. **Increase iterations:** Give creator more iterations to improve

### Loops Execute in Wrong Order
**Problem:** Loops run before their dependencies

**Solution:**

- Use `depends_on` explicitly:loop-b:
-   depends_on: \[loop-a]  # Explicit dependency
- Don't rely on `execute_loops` order alone
- Dependencies override execution order

## Performance Considerations
### Parallel Execution (Future)
Current implementation is sequential. Future enhancement could support:

```yaml
loops:
  task-a: { ... }
  task-b: { ... }  # No dependency on task-a
  task-c: { ... }  # No dependency on task-a

# Could execute task-a, task-b, task-c in parallel
```
Currently, loops execute serially even without dependencies.

### State Management
Each loop maintains its own state file:

- `~/.comanda/loop-states/data-collector.json`
- `~/.comanda/loop-states/data-analyzer.json`
- etc.

This allows individual loop resume if orchestration is interrupted.

### Memory Usage
Variables are stored in memory during orchestration:

- Large outputs → large memory usage
- Consider writing large data to files
- Use variables for metadata, file paths for bulk data

## Advanced Patterns
### Multi-Stage Validation
```yaml
workflow:
  creator:
    type: loop
    loop: implementation
    role: creator

  syntax-checker:
    type: loop
    loop: syntax-validation
    role: checker
    validates: creator
    on_fail: rerun_creator

  security-checker:
    type: loop
    loop: security-scan
    role: checker
    validates: creator
    on_fail: abort  # Security issues → stop immediately
```
### Branching Workflows
Branching workflows are supported through the `defer:` tag, which allows conditional execution:

```yaml
# Main workflow - determine the type
determine_type:
  model: claude-code
  action: |
    Analyze the input and determine the type.
    Output as JSON: {"type": "type-a"} or {"type": "type-b"}
  output: $ANALYSIS_RESULT

# Deferred branches - execute after main steps
defer:
  handle-type-a:
    input: STDIN
    model: claude-code
    action: |
      If the analysis result indicates type-a, process accordingly.
      Otherwise, output "Not applicable for type-a"
    output: STDOUT

  handle-type-b:
    input: STDIN
    model: claude-code
    action: |
      If the analysis result indicates type-b, process accordingly.
      Otherwise, output "Not applicable for type-b"
    output: STDOUT
```
Deferred steps execute after main workflow steps complete. Conditional logic is handled within the step's action prompts by checking variables and workflow state.

For more examples, see `/examples/defer-example/defer.yaml` in the Comanda repository.

