# Repository guidelines

## Documentation

- Keep all documentation concise, clear, and focused on what the reader needs.
- Use bullet points when they make information easier to scan.
- Avoid repetition and unnecessary detail.

## Conversation

- Keep explanations short and plain: lead with a brief summary, use concise bullets for behavior, and call out decisions separately. Include technical details only when needed and small examples when helpful.

## Code

- Apply DRY: reuse shared logic and avoid duplicating behavior or knowledge.
- Follow SOLID principles: keep responsibilities focused, interfaces small, and dependencies explicit.
- Prefer simple, maintainable designs; introduce abstractions when they reduce meaningful duplication or coupling.
- Aim for source and test files under 300 lines; split longer files by responsibility.

## Tool output

- Review diffs first; read targeted code sections and avoid repeated dumps.
- Summarize successful checks; keep full logs for failures.
- Print concise batch results and limit output before it needs truncating.

## Releases

- Version by scope: patch for fixes, minor for new capabilities (including `0.x`); assess compatibility before tagging.
- Record notable changes in `CHANGELOG.md` under `Unreleased` as part of each change; focus on user-visible changes.
- Follow [the release process](README.md#release-a-version); keep changelog releases newest first.
- When completing a new release, review and publish the GitHub draft created by CI, then verify the release is public.
- For release tooling changes, run `python3 -m unittest discover -s scripts -p 'test_*.py'` and validate `.github/workflows/macos.yml` with actionlint.
