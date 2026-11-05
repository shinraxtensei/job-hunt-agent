# 0019. The worker reads the profile files itself, with ruamel.yaml

- **Status:** Accepted
- **Date:** 2026-11-05
- **Story:** docs/stories/M3-05-skill-map-and-search-profile.md

## Context

The skill map and search profile (`data/profile/*.yaml`) are personal, git-ignored files
the owner edits by hand. Scoring (M3-06, Python) reads them. The story asks for:

- validation like the watch list and presets: every invalid line reported with its line
  number;
- a new version picked up without a restart.

Python has no YAML parser in its standard library. PyYAML 6.0.3 has no Python 3.15 wheel,
so it would build from source in the image. ruamel.yaml 0.19 is pure Python (its C
extension is an opt-in extra) and keeps the line of every mapping and list item.

## Decision

- **The worker reads the files directly,** from `PROFILE_DIR`: in Compose, `data/profile`
  mounted read-only at `/app/profile`. It reloads them when their content hash changes,
  checked at each loop iteration. The hashes are the profile's version, which M3-06 stores
  with every score. Go does not read them.
- **ruamel.yaml** (`typ="rt"`, the round-trip loader, for line numbers), no C extension.
  Its untyped output is converted at once into Pydantic models, so the rest of the code
  is strictly typed.
- **Validation collects every problem** (`line N: …`) before anything runs. The worker
  refuses a broken file and keeps the last good version (logged); at start-up, a broken
  file stops it.
- **Matching:** names and aliases are compared after NFKD accent folding, casefolding and
  collapsing spaces, dashes and dots. A name or alias that resolves to two skills is a
  validation error.

## Alternatives considered

- **PyYAML:** the common choice, but no cp315 wheel; building from source in the slim
  image needs a compiler (or silently drops to pure Python).
- **Validate in Go and sync into Postgres** (like the watch list): one parser for both
  languages, but the owner would have to run a sync after every edit, and personal data
  would live in two places.
- **TOML (`tomllib`, standard library):** no new dependency, but no line numbers for
  semantic errors, and the story, presets and watch list all use YAML.

## Consequences

- **Good:** one pure-Python dependency; line-numbered errors; edits apply without a
  restart.
- **Bad:** a second YAML parser in the repository (Go has yaml.v3); ruamel.yaml's API is
  loosely typed (kept to one module).
- **Revisit when:** PyYAML ships cp315 wheels and we need it elsewhere.
