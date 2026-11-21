# Slash Commands

Use the `/` prefix to execute specific commands within the TUI.

## Commands

| Command | Description | Usage |
|---------|-------------|--------|
| `/help` | Shows help information and documentation | `/help` |
| `/attach <path>` | Attach an image to the conversation | `/attach /path/to/image` |
| `/approve <id>` | Approve a pending slash command request | `/approve <uuid>` |
| `/deny <id>` | Deny a pending slash command request | `/deny <uuid>` |
| `/role` | Display or set the current role | `/role` or `/role <role_name>` |
| `/clear` | Clear the current conversation | `/clear` |
| `/undo [n]` | Undo the last `n` messages. If `n` is omitted, it defaults to 1. | `/undo` or `/undo 3` |
| `/cd <dir>` | Change the current working directory | `/cd /path/to/dir` |

## Shell Commands

You can execute shell commands directly by prefixing them with `!`. The result will be formatted and displayed in the chat.

**Example:**
`!ls -la`
