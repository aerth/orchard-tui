# Orchard-TUI (BETA)

This is a basic example of a terminal interface for [Orchard AI Agent](https://github.com/aerth/orchard).

## **Warnings**

Watch output and logs carefully!!

- Is unsafe.
- Runs shell commands.
- Is dangerous.
- Executes code provided by LLM.

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

Run with `-v` for more logs.
See `/tmp/orchard.json.log`
