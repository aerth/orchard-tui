# Orchard-TUI (BETA)

This is a **very basic** example of a terminal interface for [Orchard AI Agent](https://github.com/aerth/orchard).

## **Warnings**

This is BETA software, watch output and logs carefully!

- Runs shell commands.
- Executes code provided by LLM.

## Installation

Install easily with Go.

Since we use sqlite, you must have a C compiler. Compiling the sqlite part takes a while, be patient!

Go get:

```bash
export GOBIN=~/bin/
go install -v github.com/aerth/orchard-tui@latest
ls -l ${GOBIN}/orchard-tui
```

Or, clone and use `make`:

```bash
git clone https://github.com/aerth/orchard-tui
cd orchard-tui
make
./bin/orchard-tui
```

## Usage

llama.cpp example

```bash
export LLM_MODEL=gemma-4-E2B-it-Q4_K_M LLM_HOST=http://127.0.0.1:9931/v1/chat/completions LLM_API_KEY=keyhere
cd path/to/some/project
orchard-tui -db /tmp/orchard.sqlite 2>/tmp/orchard.log
```

ollama example

```bash
export LLM_MODEL=gemma4:12b LLM_HOST=http://127.0.0.1:11434/v1/chat/completions
cd path/to/some/project
orchard-tui -db /tmp/orchard.sqlite 2>/tmp/orchard.log
```

## Logs

See `/tmp/orchard.json.log` and run with `-v` for more logs.

## Dynamic Tools

This program reads ORCHARD_TOOLS env to create a dynamic toolbox.

If used, this replaces the default toolbox (currently only bash tool).
To use the builtin bash tool _with_ the toolbox, add `--bash` flag when opening orchard-tui.

For a nice starter pack of tools to use, try [orchard-tools](https://github.com/aerth/orchard-tools) toolbox.

```
git clone https://github.com/aerth/orchard-tools ~/src/otools
ORCHARD_TOOLS=~/src/otools orchard-tui -db /tmp/orchard.db --bash
```

### ORCHARD_TOOLS directory structure

```bash
# example file tree
${ORCHARD_TOOLS}/toolboxes/files/grep.bash
${ORCHARD_TOOLS}/toolboxes/web/web_search.bash
${ORCHARD_TOOLS}/toolboxes/web/.env
${ORCHARD_TOOLS}/.env
```

For now, env files in a toolbox is only used for scripts in that toolbox,
If no .env file exists in the inner toolbox, the main toolbox .env is used

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
