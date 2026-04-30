# SemSource e2e seed source

This file exists so the SemSource e2e config can ship with `sources` populated to one item — `len(sources) > 0` is a hard validation requirement on SemSource boot.

The `research-with-source` Playwright journey adds a real source via the `add_source_repo` tool at runtime. This file is never the test target; it just satisfies the boot check.
