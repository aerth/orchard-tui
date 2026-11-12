package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aerth/mostly/mllm"
)

func (m *model) addNewMessage(role string, content string) {
	msg := *mllm.Message{
		Role:    role,
		Content: content,
	}.ToThreadMessage(m.userName, m.threadID)
	if role == "" {
		msg.Content = ""
		msg.Aux = content
	}
	m.convo = append(m.convo, msg)

	m.renderConvo(false)
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
}

func (m *model) SlashCommandHandler(s string) (tea.Model, tea.Cmd) {
	shell, isShell := strings.CutPrefix(s, "!")
	cmd, isCmd := strings.CutPrefix(s, "/")
	if !isShell && !isCmd {
		return *m, nil
	}
	if isShell {
		c := exec.CommandContext(context.TODO(), "bash", "-c", shell)
		out, err := c.CombinedOutput()
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "(no output)"
		}
		exitcode := c.ProcessState.ExitCode()
		if err == nil && exitcode != 0 {
			err = fmt.Errorf("non-zero exit code")
		}
		if err != nil {
			output += "Error: " + err.Error()
		}
		content := fmt.Sprintf("Bash executed: %s\nExit Code %d\nOutput: %s\n", shell, exitcode, output)
		m.addNewMessage("user", content)
		return m, nil

	}
	args := strings.Fields(cmd)
	switch args[0] {
	case "help":
		m.addNewMessage("", "Help mode here")
		return m, nil
	case "undo":
		n := 1
		if len(args) != 1 {
			var err error
			n, err = strconv.Atoi(args[1])
			if err != nil {
				return m.WithError(err), nil
			}
			if n < 1 || n > len(m.convo) {
				return m.WithError(fmt.Errorf("out of range")), nil
			}
		}
		if n >= len(m.convo) {
			m.convo = nil
			m.renderConvo(true)
		} else {
			m.convo = m.convo[:len(m.convo)-n]
			m.renderConvo(false)
		}
		return m.WithMessage(fmt.Sprintf("undid %d messages now have %d", n, len(m.convo))), nil
	case "cd":
		if len(args) != 2 {
			return m.WithError(fmt.Errorf("need directory to cd")), nil
		}
		if err := os.Chdir(args[1]); err != nil {
			return m.WithError(err), nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return m.WithError(err), nil
		}
		m.addNewMessage("system", fmt.Sprintf("Changed Directory: %q (user requested)", wd))
		m.renderConvo(false)
		return m, nil

	}
	return *m, nil
}
