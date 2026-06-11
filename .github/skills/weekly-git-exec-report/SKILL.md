---
name: weekly-git-exec-report
description: 'Analyze git history for the current week across all repositories under the Projects folder, check all branches, deduplicate merged work, and produce a concise executive weekly work report. Use for weekly status reports, leadership summaries, standups, and cross-project git activity reviews, including promotion actions like pushed to test or merged to master.'
argument-hint: 'Optional: week range, repo root override, audience, or output format'
user-invocable: true
disable-model-invocation: false
---

# Weekly Git Exec Report

Create a concise executive weekly work report from git activity across all subprojects in the Projects folder.

This skill is read-only. It should inspect repositories, branches, commits, and merge activity, then summarize the week's work in business-readable language.

## When to Use
- Weekly work reports based on git history
- Executive summaries of what changed across multiple repos
- Cross-project standups for the current week
- Requests to check all branches but avoid duplicate reporting from merges
- Requests that still want branch promotion milestones called out, such as pushed to test or merged to master

## Defaults
- Root folder: `/Users/aaronstockdale/Documents/Projects`
- Report window: current calendar week, Monday 00:00 local time through now
- Scope: all git repos under the root folder
- Audience: executive or stakeholder summary
- Output style: concise bullets, no commit hashes unless the user asks

## Procedure
1. Parse any user overrides.
   - If the user specifies a different week, date range, root folder, audience, or format, use that instead of the defaults.
   - If the user says "this week" or gives no range, use Monday 00:00 local time through now.

2. Discover git repositories under the root folder.
   - Look for subprojects that contain `.git` directories.
   - Ignore obvious non-repo folders and generated folders when scanning.
   - Treat each repo as a separate source of work, then combine the results into one report.

3. Inspect all-branch activity for the time window.
   - Gather unique commits reachable from all local and remote branches for the date range.
   - Gather merge commits and branch-promotion activity separately.
   - Check branch names and decorations so release flow actions like develop -> test -> master are not missed.

4. Deduplicate work before writing.
   - Do not report the same implementation twice just because it appears on feature, develop, test, and master.
   - Deduplicate by underlying work item, not just by branch appearance.
   - Keep meaningful promotion actions as separate delivery milestones when they show progress, for example:
     - pushed to test
     - merged to master
     - published release tag

5. Classify the week's work.
   - Prioritize features, shipped capabilities, new workflows, admin tools, UI changes, public behavior changes, and release milestones.
   - Collapse related commits into a single higher-level bullet when they belong to the same feature.
   - Omit low-signal noise such as routine version bumps, dependency churn, or mechanical merges unless they mark a real release promotion.

6. Disambiguate vague commits when needed.
   - If a commit subject is unclear, inspect `git show --stat --summary <sha>` before summarizing it.
   - If several commits appear related, inspect the touched files to confirm whether they are one feature or separate items.
   - Prefer the user-facing outcome over the internal implementation detail.

7. Write the final report.
   - Start with a short heading or one-line framing only if useful.
   - Prefer 3 to 7 concise bullets total when possible.
   - Group by project only when multiple repos had meaningful work.
   - Use plain executive language such as:
     - Added server invite management for admins and managers.
     - Moved library downloads to the external browser flow.
     - Promoted the Android release through test and merged it to master.

## Decision Rules
- If a repo only has release-promotion activity, mention that only if it is meaningful delivery progress.
- If the same feature spans multiple commits, summarize it once.
- If a feature was implemented on one branch and later promoted through test or master in the same week, mention the implementation once and optionally add one delivery bullet for the promotion.
- If several repos were touched but only one has meaningful user-facing work, keep the report focused on that repo.
- If the user asks for "features only," exclude pure fixes unless they materially changed user-visible behavior.

## Quality Checks
- Every bullet is backed by unique git evidence within the requested time window.
- No duplicate bullets are caused by merges, cherry-picks, or branch syncs.
- Promotion steps like pushed to test or merged to master are preserved when relevant.
- The report is concise and readable by non-engineers.
- Internal file names, commit hashes, and code-level detail are omitted unless the user asks for them.

## Example Prompts
- `/weekly-git-exec-report this week`
- `/weekly-git-exec-report last week, features only`
- `/weekly-git-exec-report summarize this week's work across all Projects repos`
- `/weekly-git-exec-report this week, include release promotions to test and master`
