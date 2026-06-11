---
name: ralph-advanced
description: "Kick off advanced phased Ralph-loop development with machine-readable YAML plans, enforceable exit checks, atomic commits, and strict change limits. Use when: starting a new feature with disciplined phase-gated development, when you need machine-checkable exit criteria, commit hygiene, or max_files/max_lines enforcement. Trigger phrases: 'ralph-advanced', 'advanced-ralph', 'phase-gated', 'exit-checks', 'machine-readable-plan'."
---

# Ralph Advanced — Phased Development Skill

## When to Use

- Starting a new multi-phase feature that requires strict discipline
- You need machine-readable plans with enforceable exit checks
- You want atomic commits with change limits per phase
- You want the agent to self-evaluate test and linter results

## Procedure

This skill guides the user through launching the **RalphAgentAdvanced** custom agent. The agent handles the actual phased work; this skill ensures proper setup and handoff.

### Step 1 — Gather Requirements

Ask the user for:

1. **Goal**: A short, high-level description of what they want to build.
2. **Scope estimate** (optional): Roughly how many files or lines they expect.
3. **Constraints**: Tech stack, testing framework, linter, or any known blockers.

If the user hasn't provided a clear goal, use the `ask-questions` tool:

- *What feature or change are you planning?*
- *What tech stack and tools should the plan use (e.g., pytest, flake8)?*
- *Are there existing files or modules this work should build on?*

### Step 2 — Launch the Agent

Invoke the **RalphAgentAdvanced** custom agent with the user's goal as the prompt. The agent will:

1. Inspect the repository.
2. Create `PLAN.md` with a machine-readable YAML `plan-meta` block.
3. Execute Phase 1 and stop.

Example invocation:

```
Invoke RalphAgentAdvanced with: "Goal: Implement water flow simulation. Context: Python generator module, existing water.py needs flow distance algorithm, pytest for tests."
```

### Step 3 — Review the Plan (Optional)

After the agent creates `PLAN.md`, you can:

- Review the YAML `plan-meta` block for reasonable phase boundaries.
- Check that `max_files` and `max_lines` values are conservative.
- Verify exit checks are testable in the current environment.

If phases look too large, tell the agent to split them on the next invocation.

### Step 4 — Iterate

The agent will stop after each phase with:

- **Current phase:** `<id> - <name>`
- **Ready for next invocation to continue with:** `<NextInvocation task>`
- `===END_PHASE===`

To continue, invoke **RalphAgentAdvanced** again (with no new goal — the agent will read `PLAN.md` and pick up from the current phase).

## Plan Format Reference

The agent's `PLAN.md` uses this YAML structure:

```yaml
---
plan-meta:
  goal: "One-line goal"
  context: "Constraints and tech stack"
  current_phase: 1
  phases:
    - id: 1
      name: "Phase name"
      description: "What this phase delivers"
      expected_artifacts: ["file1", "file2"]
      entry_checks: ["repo:exists"]
      exit_checks: ["file:file1", "test:tests/test_file.py::test_x"]
      max_files: 5
      max_lines: 400
  next_invocation:
    - "Task for next agent run"
---
```

### Exit Check Types

| Format | Meaning |
|--------|---------|
| `file:<path>` | File must exist |
| `test:<pytest_node>` | Pytest node must pass |
| `lint:<tool>` | Linter must exit 0 |
| `grep:<regex>:<path>` | Regex must match in file |
| `cmd:<shell command>` | Command must exit 0 |
| `repo:<condition>` | Repo condition (e.g., `repo:exists`) |

## Common Patterns

### Starting a New Feature

```
"Build a chunk serialization system for the world generator"
→ RalphAgentAdvanced creates plan, scaffolds phase 1, stops.
```

### Continuing an Existing Plan

```
"Continue" or "next phase"
→ RalphAgentAdvanced reads PLAN.md, executes current_phase, stops.
```

### Adjusting Scope Mid-Flight

```
"The current phase is too big, split it"
→ RalphAgentAdvanced updates PLAN.md, splits the phase, implements the first sub-step, stops.
```

## Anti-patterns to Avoid

- **Running multiple phases in one invocation**: The agent is designed to stop after one phase. Resist the urge to continue.
- **Vague goals**: "Make it better" won't produce a good plan. Be specific.
- **Ignoring exit check failures**: If the agent reports failed checks, investigate before continuing.
- **Setting max_files too high**: Start with 3–5 files per phase; increase only if the phase genuinely requires it.

## Related Customizations

- **RalphAgentSimple** (`.github/agents/ralph-agent-simple.agent.md`) — lighter-weight version without YAML plans, exit checks, or commit discipline
- **Agent customization skill** — for editing agent files, skills, or instructions
