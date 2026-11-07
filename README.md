# Orchard-TUI (BETA)

This is a **very basic** example of a terminal interface for [Orchard AI Agent](https://github.com/aerth/orchard).

## **Warnings**

Watch output and logs carefully!!

- Is unsafe.
- Runs shell commands.
- Is dangerous.
- Executes code provided by LLM.

## Installation

Install easily with Go.

Since we use sqlite, you must have a C compiler.

Go get:

```bash
export GOBIN=~/bin/
go install -v -trimpath -ldflags '-w -s' github.com/aerth/orchard-tui@latest
ls -l ${GOBIN}/orchard-tui
```

Or, clone and use `make`:

```
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

ORCHARD_TOOLS=~/src/otools

For example: `${ORCHARD_TOOLS}/toolboxes/files/grep.bash`

The tool spec is created from the comments at the top of the script.

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
