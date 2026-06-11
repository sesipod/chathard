---
name: ralph-simple
description: "Phased Ralph-loop development with human-readable plans, one phase per invocation, and reflective logging. Use when: breaking complex features into iterative phases, working on multi-step implementation plans, enforcing one-phase-per-invocation discipline, or when you want lightweight incremental development without YAML plans or exit checks. Trigger phrases: 'ralph-simple', 'simple-ralph', 'phased', 'iterate', 'plan-driven', 'one phase at a time', 'incremental development'."
---

# Ralph Simple — Phased Development Skill

## When to Use

- Starting a new feature that should be built incrementally
- You want the agent to plan, execute one phase, log, reflect, and stop
- You prefer lightweight human-readable plans over machine-readable YAML
- You don't need enforceable exit checks, commit discipline, or change limits

For stricter discipline (YAML plans, exit checks, `max_files`/`max_lines`), use **[ralph-advanced](../ralph-advanced/SKILL.md)** instead.

## Procedure

This skill guides the user through launching the **RalphAgentSimple** custom agent. The agent handles the actual phased work; this skill ensures proper setup and handoff.

### Step 1 — Gather Requirements

Ask the user for:

1. **Goal**: A short, high-level description of what they want to build.
2. **Known constraints** (optional): Files to touch, tech stack considerations, or gotchas.

If the user hasn't provided a clear goal, use the `ask-questions` tool:

- *What feature or change are you planning?*
- *Are there existing files or modules this work should build on?*

### Step 2 — Launch the Agent

Invoke the **RalphAgentSimple** custom agent with the user's goal as the prompt. The agent will:

1. Inspect the repository.
2. Create `PLAN.md` with a human-readable plan (Goal, Context, Phases, Current Phase).
3. Execute Phase 1 and stop.

Example invocation:

```
Invoke RalphAgentSimple with: "Goal: Add ocean biome variant with deeper water levels and kelp decoration."
```

### Step 3 — Review the Plan (Optional)

After the agent creates `PLAN.md`, you can:

- Review the phase breakdown for reasonable sizing.
- Check that each phase has clear entry and exit criteria.
- Verify the agent isn't attempting too much in a single phase.

If phases look too large, tell the agent to split them on the next invocation.

### Step 4 — Iterate

The agent will stop after each phase with:

- **Current phase:** `<id> - <name>`
- **Ready for next invocation to continue with:** `<next phase description>`

To continue, invoke **RalphAgentSimple** again (with no new goal — the agent will read `PLAN.md` and pick up from the current phase).

## Plan Format Reference

The agent's `PLAN.md` uses this human-readable structure:

```markdown
# Plan: [Feature name]

## Goal
One-line description of what is being built.

## Context
Constraints, assumptions, and discovered facts.

## Phases

### Phase 1: [Name]
- **Description**: What this phase delivers.
- **Expected artifacts**: Files created or modified.
- **Entry criteria**: What must be true to start.
- **Exit criteria**: What must be true to finish.

### Phase 2: [Name]
...

## Current Phase
Phase 1: [Name]
```

## State Files

| File | Purpose |
|------|---------|
| `PLAN.md` | Goal, context, phases, current phase |
| `LOG.md` | Chronological log of each invocation's changes |
| `TODO.md` | Optional granular task breakdown |

## Common Patterns

### Starting a New Feature

```
"Refactor the noise generation module to support multiple octaves"
→ RalphAgentSimple creates plan, scaffolds phase 1, stops.
```

### Continuing an Existing Plan

```
"Continue" or "next phase"
→ RalphAgentSimple reads PLAN.md, executes current_phase, stops.
```

### Adjusting Scope Mid-Flight

```
"Phase 2 is too broad, split it into two"
→ RalphAgentSimple updates PLAN.md, splits the phase, implements the first sub-step, stops.
```

## Anti-patterns to Avoid

- **Running multiple phases in one invocation**: The agent is designed to stop after one phase. Resist the urge to continue.
- **Vague goals**: "Improve generation" won't produce a good plan. Be specific about what and why.
- **Ignoring LOG.md**: The log is the audit trail — check it if something seems off.
- **Skipping reflection**: If the agent hasn't updated the Context or adjusted future phases, prompt it to reflect.

## Ralph Simple vs Ralph Advanced

| Feature | Simple | Advanced |
|---------|--------|----------|
| Plan format | Human-readable markdown | Machine-readable YAML + markdown |
| Exit checks | Human-described criteria | Machine-checkable (`test:`, `lint:`, `file:`) |
| Change limits | No hard limits | `max_files` / `max_lines` per phase |
| Commit discipline | Optional | Enforced (1–3 commits, format-checked) |
| Exit check evaluation | Self-reported | Run via terminal commands |
| Stop token | Two explicit lines | `===END_PHASE===` token |

Choose **Simple** for exploratory work or when you trust the agent's judgment on phase sizing. Choose **Advanced** for production features where discipline and verifiability matter.

## Related Customizations

- **RalphAgentAdvanced** (`.github/agents/ralph-agent-advanced.agent.md`) — stricter version with YAML plans, exit checks, and change limits
- **RalphAdvanced skill** (`.github/skills/ralph-advanced/SKILL.md`) — companion skill for the advanced agent
- **Agent customization skill** — for editing agent files, skills, or instructions
