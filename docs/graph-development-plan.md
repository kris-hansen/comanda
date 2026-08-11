# Comanda Graph Development Plan

## Decision

Make the execution graph the first-class internal abstraction in Comanda, while
keeping today's YAML pipeline syntax fully supported. The graph is not an
LLM-invented execution plan: it is a static, inspectable, validated program
that can be versioned, charted, tested, and replayed.

The user-facing promise is:

> Comanda turns multi-agent work into a reproducible program: coordination is
> deterministic, parallelism is automatic where safe, and models are used only
> where judgement is useful.

The first product wedge is a native **diamond** pattern:

```text
scope -> fan-out specialists -> normalize/dedupe -> verifier(s) -> synthesis
```

It applies directly to code review, research, incident analysis, architecture
decisions, and vendor evaluation. It demonstrates the important distinction:
the model produces judgements; deterministic code performs normalization,
deduplication, scheduling, provenance, and policy enforcement.

## Why now

Comanda has most of the ingredients already, but they live in separate paths:

- sequential steps and `parallel-process` groups;
- file-backed fan-in and dependency validation;
- named agentic-loop DAGs, including cycle detection and topological ordering;
- quality gates, worktrees, model selection, progress reporting, and charts.

This means the project does **not** need to invent an agent framework. It needs
one execution model that exposes those capabilities consistently. In
particular, the current loop DAG is scheduled in a topological *sequence*, and
the ordinary workflow path treats parallelism as a syntactic group. Neither is
yet a general ready-node scheduler with explicit data contracts and edges.

## Product principles

1. **Graphs are declarative and static at run start.** A generator may choose
   and fill a template, but its generated graph is shown, validated, and saved
   before it runs. Runtime LLMs cannot silently add arbitrary nodes or tools.
2. **Edges carry declared artifacts, not ambient prompt state.** A node reads
   named, persisted artifacts from incoming edges. This makes provenance,
   replay, caching, and inspection possible.
3. **Use deterministic code for mechanical work.** Normalization, schema
   validation, dedupe, artifact handling, fan-in, and scheduler decisions must
   be Go components, not another prompt.
4. **Schedule all ready nodes concurrently.** A node waits only for the gates
   on its incoming edges; it does not wait for an unrelated branch or an
   entire stage unless the graph says so.
5. **Compatibility is a product requirement.** Existing linear YAML,
   `parallel-process`, `defer`, loops, file artifacts, and `comanda process`
   must retain their documented behavior during the migration.
6. **Failure behavior is explicit.** Each node and edge declares whether to
   fail fast, retry, skip, continue with a partial aggregate, or require human
   intervention. There must be no accidental "best effort" synthesis.
7. **Every result is traceable.** Persist node inputs, outputs, schema/gate
   outcomes, model/provider metadata, duration, cost when available, and the
   graph version under the runtime directory.

## Target architecture

```text
YAML (legacy or graph) -> parser/lowering -> validated Graph IR -> planner
                                                        |               |
                                                        v               v
                                                  artifact plan    ready-node scheduler
                                                                        |
                                                                        v
      run record <- events/provenance <- executors (LLM, tool, Go transform, loop)
                                                                        |
                                                                        v
                                 chart / logs / replay / final declared artifacts
```

### 1. Graph IR

Introduce an internal `Graph` representation independent of YAML syntax. It
contains stable node IDs, typed ports, edges, policies, resource hints, and a
graph version. The IR is the only form accepted by the planner and executor.

Lower existing workflows into it:

- a sequential `STDOUT -> STDIN` chain becomes explicit sequential edges;
- a file output/input pair becomes an explicit artifact edge;
- a `parallel-process` group becomes independent nodes with no artificial
  dependencies between them;
- a loop remains a node executor initially; its internal orchestration stays
  intact until it can be safely lowered further;
- `defer` stays a conditional subgraph and is out of the first execution slice.

This gives the migration one source of truth while preserving the existing DSL.

### 2. Artifact and contract model

A port describes an artifact, not merely a string prompt:

- `name` and direction (`input` or `output`);
- media/format (`text`, `json`, `markdown`, `file`, `patch`, etc.);
- optional JSON Schema or file-based schema reference;
- required/optional and cardinality (`one` or `many`);
- retention and sensitivity classification;
- provenance: producer node, content digest, timestamp, and validation state.

The first implementation should support `text`, `markdown`, and `json`, with
JSON Schema validation when a schema is declared. Existing files remain the
physical artifact store; do not introduce a database requirement. Resolve
runtime artifacts under `.comanda/runs/<run-id>/artifacts/`, while preserving
documented user output paths through a final materialization step.

### 3. Edges and gates

An edge maps one source port to one destination port and controls when the
consumer may run. Start with these gates:

- `success` (default): producer completed and artifact contract passed;
- `all_success`: a many-input fan-in waits for all required producers;
- `allow_partial`: consumer may run after its required quorum succeeds, and
  receives an explicit list of missing/failed artifacts;
- `manual`: stop before crossing the edge until a user approves or resumes.

Do not implement arbitrary expressions or model-written gates in v1. A simple,
visible gate vocabulary is much easier to reason about and test.

### 4. Ready-node scheduler

The planner validates the DAG and builds reverse dependency indexes. The
scheduler then:

1. marks root nodes ready;
2. runs every ready node subject to global and resource-class concurrency
   limits;
3. atomically records the attempt and its produced artifacts;
4. evaluates only affected outgoing edges;
5. releases newly-ready consumers, or applies their declared failure policy;
6. exits with a complete run record, including skipped and blocked nodes.

Ordering must be deterministic for equal-priority work (stable node-ID order),
although execution is concurrent. Resource classes (`fast`, `standard`,
`expensive`, `exclusive`) are admission-control hints, not a correctness
mechanism. Per-provider/model concurrency and a workflow budget can be added
once the core scheduler is proven.

### 5. Executors

Use a small executor interface behind graph nodes:

- `llm`: existing model invocation;
- `tool`: existing allowlisted tool execution;
- `transform`: trusted built-in Go transforms, starting with JSON
  normalization and finding deduplication;
- `loop`: existing agentic-loop execution as an opaque node;
- later: `router`, `approval`, and `subworkflow`.

All executors receive resolved input artifacts and write declared output
artifacts. They never choose downstream nodes directly.

## Proposed graph DSL (design target, not first shipping parser)

Use explicit ports and edges rather than string interpolation between steps.
The syntax below is intentionally compact, but its exact field names should be
settled after the Graph IR and fixtures exist.

```yaml
version: comanda.graph/v1

inputs:
  change:
    format: text

nodes:
  scope:
    kind: llm
    model: gpt-4o
    action: Produce a review plan and bounded file/module scope as JSON.
    inputs:
      change: { format: text }
    outputs:
      plan: { format: json, schema: ./schemas/review-plan.json }

  security:
    kind: llm
    model: claude-sonnet-4-20250514
    action: Review the scoped change for security findings. Return finding JSON.
    inputs:
      plan: { format: json }
    outputs:
      findings: { format: json, cardinality: many, schema: ./schemas/finding.json }
    policy: { failure: retry }
    resources: { class: standard }

  correctness:
    kind: llm
    model: openai-codex
    action: Review the scoped change for correctness and regressions. Return finding JSON.
    inputs:
      plan: { format: json }
    outputs:
      findings: { format: json, cardinality: many, schema: ./schemas/finding.json }

  normalize:
    kind: transform
    transform: findings.normalize_dedupe
    inputs:
      candidates: { format: json, cardinality: many }
    outputs:
      findings: { format: json, schema: ./schemas/findings.json }

  verify:
    kind: llm
    model: gpt-4o
    action: Verify each normalized finding against the scoped change. Return verdict JSON.
    inputs:
      plan: { format: json }
      findings: { format: json }
    outputs:
      verdicts: { format: json, schema: ./schemas/verdicts.json }

  synthesize:
    kind: llm
    model: claude-sonnet-4-20250514
    action: Produce a concise review using accepted verdicts only.
    inputs:
      verdicts: { format: json }
    outputs:
      review: { format: markdown }

edges:
  - { from: input.change, to: scope.change }
  - { from: scope.plan, to: security.plan }
  - { from: scope.plan, to: correctness.plan }
  - { from: security.findings, to: normalize.candidates }
  - { from: correctness.findings, to: normalize.candidates }
  - { from: scope.plan, to: verify.plan }
  - { from: normalize.findings, to: verify.findings, gate: all_success }
  - { from: verify.verdicts, to: synthesize.verdicts }

outputs:
  review: synthesize.review
```

The `findings.normalize_dedupe` transform is deliberately a first-party Go
component. It must emit a stable fingerprint, retain all source references,
and record both retained and rejected/merged findings so a verifier and a human
can audit it.

## Delivery phases

### Phase 0 — contracts, fixtures, and baseline (1 short milestone)

Define the Graph IR, node/edge lifecycle, failure-state matrix, and artifact
manifest format in code and documentation. Add fixtures for linear, fan-out,
fan-in, failure, and cycle cases. Capture current behavior with integration
tests before changing execution.

Exit criteria:

- a written compatibility matrix maps each current DSL feature to its migration
  path;
- fixtures define expected topology, run state, and output behavior;
- existing test suite is green with no runtime behavior changed.

### Phase 1 — lower legacy workflows and run the graph internally

Implement `Graph`, `Node`, `Port`, `Edge`, `Artifact`, and `RunRecord` types.
Lower ordinary sequential/file-backed workflows and existing parallel groups
into the IR. Route them through a graph planner, but retain legacy execution as
the comparison oracle behind a feature flag.

Initially support static success-only edges, persisted artifacts, cycle/missing
producer/type validation, and a bounded ready-node scheduler. Keep `defer` and
complex loop orchestration on their current paths, represented as opaque nodes
only where necessary.

Exit criteria:

- the same fixture can run through legacy and graph paths with equivalent
  outputs and error semantics;
- independent nodes overlap in time; dependent nodes do not start early;
- graph execution has stable ordering, cancellation, cleanup, and a full run
  record;
- `comanda chart` renders the lowered edges correctly.

### Phase 2 — ship the native diamond template

Add the first user-facing graph template and built-in JSON transforms:
normalization, stable fingerprinting, dedupe, and merge provenance. Add schema
validation at node output boundaries and `all_success`/`allow_partial` fan-in
gates. Provide a copy-ready code-review workflow plus a deterministic fixture.

Exit criteria:

- a three-specialist diamond schedules its specialists concurrently;
- duplicate findings merge deterministically with all source IDs retained;
- invalid output fails at the producing node with a clear contract error;
- partial-mode synthesis explicitly identifies absent/failed branches;
- chart, JSON run record, and terminal progress all expose node status,
  artifact paths, gate results, model, and duration.

### Phase 3 — policies, budget, and observability

Add explicit retry/backoff, failure policies, resource/provider concurrency,
timeouts, token/cost accounting where providers expose it, cache keys, and
resume/replay of completed nodes. Quality gates become graph nodes or edge
gates with the same run-record semantics rather than a loop-only feature.

Exit criteria:

- a run can resume without re-running valid, cacheable ancestors;
- policy decisions and budget stops are visible and reproducible;
- no provider or resource class can exceed its declared concurrency limit.

### Phase 4 — selected templates and guarded generation

Add inspectable templates for `pipeline`, `diamond`, `router`, and
`verify-and-retry`. Extend `comanda generate` to select and fill a template,
then require graph validation and a rendered chart before execution. Dynamic
routing is limited to declared destinations and contracts.

Exit criteria:

- generated graph files are ordinary, reviewable YAML checked into source
  control;
- generated graphs cannot create undeclared tools, outputs, or destinations;
- every template has validation fixtures and an example.

## Implementation map

| Area | Initial change |
| --- | --- |
| `utils/processor/types.go` | Add Graph IR, ports, edges, artifacts, policies, and run-record types. Keep `StepConfig` intact. |
| `utils/processor/dsl.go` | Parse/lower legacy steps to Graph IR; dispatch the graph executor behind a flag, then make it the default after parity. |
| `utils/processor/graph_*.go` (new) | Validation, planning, ready-node scheduling, state transitions, artifact persistence, and executor adapters. |
| `utils/processor/quality_gates.go` | Adapt gates to graph-node/edge semantics in Phase 3; do not duplicate gate logic. |
| `utils/processor/loop_orchestrator.go` | Preserve it first; later share the DAG scheduler rather than maintaining two schedulers. |
| `cmd/chart.go` | Render IR edges, ports, gates, node state, and provenance; preserve current output formats. |
| `examples/graph/` (new) | Diamond, partial fan-in, contract-failure, and replay examples. |
| `docs/` | Graph spec, migration guide, failure-policy reference, and template authoring guide. |
| tests | Golden topology tests, scheduler race tests, contract tests, parity tests, and integration fixtures. |

## Work breakdown

1. **Graph model and validator** — types, YAML-independent validation, cycle
   reporting, artifact contracts, and golden tests.
2. **Legacy lowering** — translate sequential/file/parallel workflows with a
   compatibility fixture suite.
3. **Scheduler and run record** — bounded concurrency, cancellation, stable
   ordering, state transitions, and event/progress integration.
4. **Executor adapters** — wrap existing LLM/tool/loop behavior without
   changing provider semantics.
5. **Chart and inspectability** — graph-aware Mermaid/ASCII output plus JSON
   run manifest.
6. **Diamond transforms and template** — normalize/dedupe implementation,
   schemas, examples, and end-to-end tests.
7. **Policies and resumption** — retries, partial fan-in, budgets, caches,
   replay, and quality-gate unification.

Each item should land as a small PR with its own fixtures. Do not combine the
new DSL, scheduler rewrite, transforms, and template generator in one change.

## Compatibility and migration rules

- Existing workflow files remain valid and keep their documented stdout/file
  behavior.
- Existing `output: STDOUT` to `input: STDIN` and file-to-file flow remain
  valid legacy syntax. Native graphs use ports and edges; they do not repurpose
  `$VARIABLE` as general data flow.
- Default execution remains bounded and deterministic. Concurrency is expanded
  only when the graph proves that no dependency/gate exists.
- Existing output paths remain user-visible outputs. Internal run artifacts are
  additive and can be disabled only after equivalent debugging support exists.
- Generated workflows are never auto-executed merely because generation
  succeeded; they must parse, validate, and be visible to the user.

## Key decisions to keep deliberately narrow

Do now:

- static DAGs;
- file-backed artifacts plus contracts;
- one shared scheduler;
- deterministic transform nodes;
- the code-review diamond.

Defer:

- LLM-authored arbitrary runtime graph mutation;
- distributed execution or a database-backed artifact system;
- a visual graph editor;
- arbitrary edge expressions;
- automatic semantic dedupe by an LLM;
- turning every agentic loop into a general graph immediately.

## Success measures

- A graph chart answers: what ran, why it was eligible, what it consumed, what
  it produced, and why any node was blocked or skipped.
- For a diamond workflow, elapsed time approaches the longest specialist path,
  not the sum of specialist paths.
- A rerun can reuse valid upstream artifacts and reproduce the same graph plan.
- A code-review output links each accepted/rejected finding to its source,
  normalization decision, verification verdict, and final synthesis input.
- Existing non-graph examples and integration tests continue to pass throughout
  the migration.

## First implementation slice

Start with the smallest useful vertical slice:

1. implement the internal Graph IR and lower only regular sequential steps,
   explicit file edges, and one `parallel-process` group;
2. run that graph with success-only dependencies and persisted run artifacts;
3. add a graph-aware `comanda chart` view and parity tests;
4. add the diamond example with two specialists, a deterministic Go
   normalizer/deduper, one verifier, and one synthesizer.

That proves the thesis without committing to a broad new language. Once that
slice is boringly reliable, native graph YAML can become an opt-in surface and
the remaining legacy paths can migrate one at a time.
