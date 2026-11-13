# Orchard-TUI (BETA)

This is a **very basic** example of a terminal interface for [Orchard AI Agent](https://github.com/aerth/orchard).

Markdown is nicely "rendered" to the terminal window, this can be toggled with "shift+tab".

## Installation

Install easily with Go.

Since we use sqlite for persistence, you must have a C compiler. Compiling the sqlite part takes a while, be patient!

Otherwise, orchard-tui is a single executable. You can install it where you prefer.
Below are a few ways one might install to `~/bin/orchard-tui`

Go get:

Install to ~/bin/ directory
```bash
GOBIN=~/bin go install -v github.com/aerth/orchard-tui@latest
~/bin/orchard-tui
```

Or, the right way. Clone and use `make`:

```bash
git clone https://github.com/aerth/orchard-tui
cd orchard-tui
make && make install
~/bin/orchard-tui
```

## Usage

Minimally, `LLM_MODEL` and `LLM_HOST` is required. Without `ORCHARD_TOOLS`, a basic shell tool is the only tool available to the model. If `LLM_API_KEY` is present, it is added to Authentication header for each llm request.

Use `-h` to show command-line flag usage, and from the chat you can use `/help` to show slash commands and key bindings.

### Configuration 

Instead of always using flags and export env to run `orchard-tui`, consider a `~/.config/orchard/env` file.

In `~/.config/orchard/env`:

```bash
# env config for orchard (https://github.com/aerth/orchard)
LLM_HOST='https://ollama.home.arpa'
LLM_MODEL=gemma4:12b
LLM_API_KEY=abcdef123456
LLM_THINK=low
# if ORCHARD_TOOLS is used and this directory does not exist, the program will not start.
#ORCHARD_TOOLS=~/src/otools
# if ORCHARD_DB is used, persistence is activated.
#ORCHARD_DB=~/.local/state/orchard/orchard.db
```

### llama.cpp example

```bash
export LLM_MODEL=gemma-4-E2B-it-Q4_K_M LLM_HOST=http://127.0.0.1:9931/v1/chat/completions LLM_API_KEY=keyhere
export ORCHARD_DB=/tmp/orchard.sqlite
cd path/to/some/project
orchard-tui
```

### ollama example

```bash
export LLM_MODEL=gemma4:12b LLM_HOST=http://127.0.0.1:11434/v1/chat/completions
export ORCHARD_DB=/tmp/orchard.sqlite
cd path/to/some/project
# specifying thread allows grouping multi-directory conversations in one thread.
orchard-tui
```

## Persistence (conversation history)

When using a database for persistent conversations, we use `$PWD` as thread ID (as if thread flag was `orchard-tui -thread $PWD`) to automatically create a "thread ID" for each project directory. To specify the conversation thread ID, use the `-thread` flag. If thread argument is not a UUID, then it is hashed. This allows for words such as `-thread foo` to make it easy to use new threadID.

## Logs

See `/tmp/orchard.json.log` and run with `-v` for more logs.

## Dynamic Tools

This program reads ORCHARD_TOOLS env to create a dynamic toolbox.

When used, this *adds* to the default tools (`run_bash`). To use **only** the toolboxes _without_ the builtin bash tool, add `--no-bash` flag when opening orchard-tui.

For a nice starter pack of tools to use, try [orchard-tools](https://github.com/aerth/orchard-tools) toolbox.

```
git clone https://github.com/aerth/orchard-tools ~/src/otools
ORCHARD_TOOLS=~/src/otools orchard-tui -db /tmp/orchard.db
```

### ORCHARD_TOOLS directory structure

```bash
# example file tree
${ORCHARD_TOOLS}/toolboxes/files/grep.bash
${ORCHARD_TOOLS}/toolboxes/web/web_search.bash
```

For now, env files in a toolbox is only used for scripts in that toolbox,
If no .env file exists in the inner toolbox, the main toolbox .env is used

```bash
# optional env files
# used for 'web' toolbox
${ORCHARD_TOOLS}/toolboxes/web/.env
# used for any toolbox without their own .env file
${ORCHARD_TOOLS}/.env
```

The tool spec is created from the comments at the top of the script.

For now, the toolbox subdirectories are for organization only and all toolboxes are combined into one for LLM.

```bash
# Description: Use grep (ripgrep) to find line-numbers and filenames for keywords..
# Param: phrase - The text to search for [string] [required]
# Param: path - Path to the file (if omitted, all subdirectories) [string] [optional]
# Param: case_insensitive - Add -i flag if we arent sure about spelling. Causes more false positives! [boolean] [optional]
flags=(--line-number --word-regexp)
if [[ "$3" == "true" ]]; then
	flags+=("--ignore-case")
fi
rg "${flags[@]}" "${1}" "${2:-.}"
```

If the script spec is formatted correctly, the tool will be available to use.
The tool calling is fully abstracted, isolating the llm from the execution logic and tool implementation.

## Copyright

Copyright (c) 2026 aerth

See [LICENSE](./LICENSE) file.
