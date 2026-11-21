# Help Flags

The following flags are available for the Orchard TUI:

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-C value` | Switch directory first | |
| `-change-dir string` | Change directory before anything (default `/home/username/src/orchard-tui`) | `/home/username/src/orchard-tui` |
| `-compact int` | Auto-compact convo after it grows to `n` messages (0 disable) | `20` |
| `-compact-toolout-size int` | Compact all tool outputs that are larger than `n` characters (0 disable) | `1000` |
| `-db string` | Path to SQLite DB file (optional, for persistence. In-memory usage: `file::memory:?cache=shared`) | |
| `-e string` | Use a different env file (or `none`) instead of `~/.config/orchard/env` | |
| `-guardian-allow value` | Additional text to help bash guardian identify allowed commands, may be used multiple times | |
| `-hide-think` | Hide thinking when thinking is enabled | |
| `-hide-tools` | Hide tools, not recommended. Prefer redirecting stderr | |
| `-host string` | Host URL ONLY if no env var | |
| `-limit-history int` | Limit history (convo) given to LLM | `20` |
| `-limit-rounds int` | Limit rounds (turns) in the loop (-1 infinite) | `-1` |
| `-log-level value` | `debug`, `info`, `warn`, `error` | |
| `-model string` | Model ONLY if no env var | |
| `-no-bash` | Remove bash tool, only use toolboxes | |
| `-q` | Disable log | |
| `-require-confirm value` | Add one or comma/space-separated list of tools to be allowed but confirmation is needed. May be used multiple times. | |
| `-smoke` | Say hello, list tools available, and exit (no tool calling) | |
| `-temperature float` | Temperature | `1` |
| `-think value` | Think value, such as `low` or `none`, this is **not checked** before sending to LLM | |
| `-thread value` | Thread ID (if applicable) | |
| `-tool value` | Add one or comma/space-separated list of tools to be allowed. May be used multiple times. | |
| `-toolhook value` | Add tool hook (can be used multiple times) in format `name:command`, for example: `apply_patch_id:make lint` | |
| `-v` | Enable log | |
| `-version` | Show version info | |
