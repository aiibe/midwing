# Repository guidelines

## Documentation

- Keep all documentation concise, clear, and focused on what the reader needs.
- Use bullet points when they make information easier to scan.
- Avoid repetition and unnecessary detail.

## Code

- Apply DRY: reuse shared logic and avoid duplicating behavior or knowledge.
- Follow SOLID principles: keep responsibilities focused, interfaces small, and dependencies explicit.
- Prefer simple, maintainable designs; introduce abstractions when they reduce meaningful duplication or coupling.

## Releases

- Version by scope: patch for fixes, minor for new capabilities (including `0.x`); assess compatibility before tagging.
- Record notable changes in `CHANGELOG.md` under `Unreleased` as part of each change.
- Follow [the release process](README.md#release-a-version); keep changelog releases newest first.
- For release tooling changes, run `python3 -m unittest discover -s scripts -p 'test_*.py'` and validate `.github/workflows/macos.yml` with actionlint.
