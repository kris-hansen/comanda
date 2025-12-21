# Multi-Agent Workflows

These examples demonstrate how to leverage multiple agentic coding tools together to get better results than any single agent alone.

## Supported Agents

| Agent | Model Name | Strengths |
|-------|------------|-----------|
| **Claude Code** | `claude-code` | Deep reasoning, nuanced trade-off analysis, synthesis |
| **Gemini CLI** | `gemini-cli` | Broad knowledge, industry patterns, Google-scale solutions |
| **OpenAI Codex** | `openai-codex` | Implementation focus, code structure, practical guidance |

## Workflow Patterns

### 1. Parallel Analysis → Synthesis (`architecture-planning.yaml`)

All agents analyze the problem simultaneously, then results are synthesized:

```
┌─────────────────┐
│     Input       │
└────────┬────────┘
         │
    ┌────┴────┐
    ▼    ▼    ▼
┌──────┐┌──────┐┌──────┐
│Claude││Gemini││Codex │  ← Parallel
└──┬───┘└──┬───┘└──┬───┘
   │       │       │
   └───────┼───────┘
           ▼
    ┌────────────┐
    │  Synthesis │
    └──────┬─────┘
           ▼
    ┌────────────┐
    │   Output   │
    └────────────┘
```

**Best for:** Complex problems needing multiple perspectives

**Usage:**
```bash
echo "Build a real-time collaborative document editor" | comanda process architecture-planning.yaml
```

### 2. Sequential Refinement (`architecture-review.yaml`)

Each agent builds upon and improves the previous agent's work:

```
┌─────────────────┐
│     Input       │
└────────┬────────┘
         ▼
┌─────────────────┐
│  Claude: Draft  │
└────────┬────────┘
         ▼
┌─────────────────┐
│ Gemini: Review  │
└────────┬────────┘
         ▼
┌─────────────────┐
│ Codex: Finalize │
└────────┬────────┘
         ▼
┌─────────────────┐
│     Output      │
└─────────────────┘
```

**Best for:** Iterative improvement, catching blind spots

**Usage:**
```bash
echo "Microservices platform for e-commerce" | comanda process architecture-review.yaml
```

### 3. Voting / Consensus (`architecture-decision.yaml`)

All agents independently evaluate options, then vote:

```
┌─────────────────┐
│    Decision     │
└────────┬────────┘
         │
    ┌────┴────┐
    ▼    ▼    ▼
┌──────┐┌──────┐┌──────┐
│Vote 1││Vote 2││Vote 3│  ← Independent
└──┬───┘└──┬───┘└──┬───┘
   │       │       │
   └───────┼───────┘
           ▼
    ┌────────────┐
    │  Consensus │
    └──────┬─────┘
           ▼
    ┌────────────┐
    │    ADR     │
    └────────────┘
```

**Best for:** Specific decisions (technology choices, patterns)

**Usage:**
```bash
echo "Should we use PostgreSQL or MongoDB for our user data?" | comanda process architecture-decision.yaml
```

## Prerequisites

Ensure all three CLI tools are installed and configured:

```bash
# Claude Code
npm install -g @anthropic-ai/claude-code
export ANTHROPIC_API_KEY=your-key

# Gemini CLI
npm install -g @anthropic-ai/gemini-cli  # or via pip
export GEMINI_API_KEY=your-key

# OpenAI Codex
npm install -g @openai/codex
export OPENAI_API_KEY=your-key
```

Verify installation:
```bash
which claude gemini codex
```

## Customization

### Using Different Model Variants

Each agent supports multiple model variants:

```yaml
# Claude Code variants
model: claude-code        # Default
model: claude-code-opus   # Most capable
model: claude-code-sonnet # Balanced
model: claude-code-haiku  # Fastest

# Gemini CLI variants
model: gemini-cli         # Default
model: gemini-cli-pro     # Most capable
model: gemini-cli-flash   # Faster

# OpenAI Codex variants
model: openai-codex       # Default
model: openai-codex-o3    # Reasoning model
model: openai-codex-gpt-4.1 # GPT-4.1
```

### Creating Custom Multi-Agent Workflows

Basic pattern:
```yaml
parallel-process:
  agent-one:
    input: STDIN
    model: claude-code
    action: "Your prompt here"
    output: $AGENT_ONE_RESULT

  agent-two:
    input: STDIN
    model: gemini-cli
    action: "Your prompt here"
    output: $AGENT_TWO_RESULT

  agent-three:
    input: STDIN
    model: openai-codex
    action: "Your prompt here"
    output: $AGENT_THREE_RESULT

combine-results:
  input: |
    Agent 1: $AGENT_ONE_RESULT
    Agent 2: $AGENT_TWO_RESULT
    Agent 3: $AGENT_THREE_RESULT
  model: claude-code
  action: "Synthesize the above into a final answer"
  output: STDOUT
```

## Why Multi-Agent?

Each AI has different training data, architectures, and tendencies:

- **Diverse perspectives** reduce blind spots
- **Cross-validation** catches errors
- **Specialized strengths** combined produce better results
- **Consensus building** increases confidence in decisions

The synthesis step is key - it's not just concatenation, but intelligent combination of the best ideas from each agent.
