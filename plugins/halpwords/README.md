# Halpwords plugin for Claude Code

Sub-agents for working on the game and on
[halpwords-server](https://github.com/halpworld/halpwords-server). Both repos
switch the plugin on in `.claude/settings.json`, so Claude Code offers to
install it when you open either one.

| Agent | Use it for |
|---|---|
| `halpwords:contract-keeper` | Anything that crosses the game–server line: the shared packages, the word list format, `/api/v1` and the link client. |
| `halpwords:language-reviewer` | Word lists, riddles and gap-fills in French, Latin, Ancient Greek and Irish, and answer-matching rules. |
| `halpwords:privacy-safety-reviewer` | Children's privacy and safety before merging (read only). |
| `halpwords:llm-engineer` | Prompts, schemas, validators, fixtures and fallbacks for AI features. |
| `halpwords:balance-analyst` | Formulas, stats, scores and rankings, checked with `make balance`. |
| `halpwords:plan-keeper` | The next task, checking a branch against its task, and keeping PLAN.md and WAVES.md true. |
| `halpwords:ui-reviewer` | Game screens and server pages: accessibility, clarity, light and dark. |

Agents that look at both repos expect them side by side, as
`…/halpwords` and `…/halpwords-server`.

To change an agent, edit it here and bump `version` in
`.claude-plugin/plugin.json`; the change reaches everyone once it is on
`main` and they run `claude plugin update halpwords@halpwords`.
