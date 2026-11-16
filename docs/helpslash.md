# Slash Commands

Slash commands in the TUI are prefixed with `/` (or `!` for shell execution).

## Shell Commands

| Command | Description |
| :--- | :--- |
| `!<command>` | Execute `<command>` as a bash command and display the output |

## Slash Commands

| Command | Description |
| :--- | :--- |
| `/help` | Show help information, including flags and other slash commands |
| `/undo [n]` | Undo the last `n` messages. If `n` is not specified, undoes the last message |
| `/cd <directory>` | Change the current working directory to `<directory>` |
