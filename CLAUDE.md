# go-pinch

Go library and CLI for reading single files out of remote zip archives over HTTP range requests.

## Code health
- Install the pinned tool once per clone (needs Python 3.11+):
  `python3.12 -m venv .venv && .venv/bin/pip install "git+https://github.com/epatel/ai-health@v0.3.1"`
- Before committing, run `.venv/bin/aihealth delta` and fix new or worsened smells it
  reports in code you touched (exit code 1 = health degraded).
- `.venv/bin/aihealth scan .` gives an overview of the codebase.
