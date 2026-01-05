# Tool Use in Comanda

Execute shell commands within your workflows. Use external CLIs, filter data with `jq` and `grep`, or integrate with tools like `bd` (beads).

## Quick Start

### Tool as Input

Run a command and use its output as step input:

```yaml
list_files:
  input: "tool: ls -la"
  model: claude-code
  action: "Summarize this directory listing"
  output: STDOUT
```

### Tool as Output

Pipe LLM output through a command:

```yaml
filter_json:
  input: data.json
  model: claude-code
  action: "Return the data as JSON"
  output: "tool: jq '.items[] | select(.active)'"
```

### Pipe Previous Output

Use `STDIN|` to pipe the previous step's output through a command:

```yaml
step_one:
  input: data.json
  model: NA
  action: NA
  output: STDOUT

step_two:
  input: "tool: STDIN|grep 'error'"
  model: NA
  action: NA
  output: STDOUT
```

## Using Custom CLIs (like `bd`)

Comanda supports external CLI tools. The `bd` (beads) issue tracker is included in the default allowlist:

```yaml
# List issues
list_issues:
  input: "tool: bd list --json"
  model: NA
  action:
    - Pass through the JSON data
  output: STDOUT
  tool:
    allowlist: [bd]

# Create an issue
create_issue:
  input: NA
  model: claude-code
  action: |
    Generate a bd create command for: "Implement user authentication"
    Format: bd create --title="..." --type=feature
    Return ONLY the command, no explanation.
  output: "tool: STDIN"
  tool:
    allowlist: [bd]
```

For custom tools not in the default allowlist, add them explicitly:

```yaml
my_step:
  input: "tool: my-custom-cli --flag"
  tool:
    allowlist: [my-custom-cli]
```

## Global Configuration

Instead of specifying `tool:` blocks in every workflow, you can configure global tool settings in `~/.comanda/config.yaml`:

```yaml
# ~/.comanda/config.yaml
tool:
  allowlist:
    - bd           # Add your custom CLIs
    - mytool
  denylist: []     # Additional commands to block
  timeout: 60      # Default timeout in seconds
```

Configure interactively:
```bash
comanda configure
# Select "4. Tool Settings"
```

**Precedence:**
1. Step-level `allowlist` replaces global (if specified)
2. Denylists are always merged (global + step)
3. Step timeout overrides global timeout

## Security

### Default Allowlist (Safe Commands)

These commands are allowed by default:
- **File inspection:** `ls`, `cat`, `head`, `tail`, `file`, `stat`, `wc`
- **Text processing:** `grep`, `awk`, `sed`, `cut`, `tr`, `sort`, `uniq`, `diff`
- **Data formats:** `jq`, `yq`, `base64`, `md5sum`, `sha256sum`
- **System info:** `date`, `env`, `pwd`, `whoami`, `hostname`, `uname`
- **Issue tracking:** `bd` (beads)

### Default Denylist (Blocked Commands)

These commands are always blocked:
- **Destructive:** `rm`, `rmdir`, `mv`, `dd`, `shred`
- **Privilege escalation:** `sudo`, `su`, `doas`, `pkexec`
- **System modification:** `chmod`, `chown`, `chgrp`
- **Shell spawning:** `bash`, `sh`, `zsh`, `fish`
- **Network:** `curl`, `wget`, `ssh`, `scp`
- **Package managers:** `apt`, `npm`, `pip`, `brew`, `cargo`

### Step-Level Configuration

Override defaults per-step:

```yaml
my_step:
  input: "tool: my-command"
  tool:
    allowlist: [my-command, grep, jq]  # Replaces default allowlist
    denylist: [rm]                      # Adds to default denylist
    timeout: 60                          # Seconds (default: 30)
```

**Warning:** Custom `allowlist` completely replaces the default. Include all needed commands.

## Examples

### List Directory and Summarize

```yaml
summarize_dir:
  input: "tool: ls -la"
  model: claude-code
  action: "Create a summary of this directory"
  output: STDOUT
```

### Filter JSON with jq

```yaml
active_users:
  input: users.json
  model: NA
  action: NA
  output: "tool: jq '.[] | select(.active == true)'"
  tool:
    allowlist: [jq]
```

### Chain Commands

```yaml
count_errors:
  input: "tool: cat /var/log/app.log | grep ERROR | wc -l"
  model: claude-code
  action: "Report on the error count"
  output: STDOUT
  tool:
    allowlist: [cat, grep, wc]
```

### Beads Integration

See `beads-workflow-example.yaml` and `beads-walkthrough.yaml` for complete examples of integrating with the beads issue tracker.

## Example Files

- `tool-input-example.yaml` - Using commands as step input
- `tool-output-example.yaml` - Piping output through commands
- `beads-workflow-example.yaml` - Basic beads (bd) integration
- `beads-walkthrough.yaml` - Walkthrough: spec analysis → issue preview
- `conveyor-belt-spec-to-beads.yaml` - Full automation: spec → bd create execution
- `sample-spec.md` - Sample technical spec for testing the conveyor belt

### Running the Conveyor Belt

Analyze a spec and automatically create beads issues:

```bash
# Using the sample spec
cat examples/tool-use/sample-spec.md | comanda process examples/tool-use/conveyor-belt-spec-to-beads.yaml

# Using your own spec
cat my-feature-spec.md | comanda process examples/tool-use/conveyor-belt-spec-to-beads.yaml
```

This will:
1. Analyze the spec and break it into ~1000 LOC tasks
2. Create beads issues for each task (up to 5)
3. Show a summary of created issues

## Troubleshooting

**"Command not allowed" error:**
- Check if the command is in the default denylist
- Add it to the step's `allowlist`

**"Command timed out" error:**
- Increase `timeout` in the tool config (default: 30 seconds)

**Command not found:**
- Ensure the CLI is installed and in your PATH
- Test the command directly in terminal first
