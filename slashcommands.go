package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"uuid"

	tea "charm.land/bubbletea/v2"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard-tui/pkg/roles"
)

func (m *model) addNewM(msg mllm.ChatMessage) {
	m.convo = append(m.convo, msg)
	m.renderConvo(false)
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
}

func (m *model) addNewMessage(role string, content string) {
	msg := *mllm.Message{
		Role:    role,
		Content: content,
	}.ToThreadMessage(m.userName, m.threadID)
	if role == "" {
		msg.Content = ""
		msg.Aux = content
	}
	m.addNewM(msg)
}

func (m *model) SlashCommandHandler(s string) (tea.Model, tea.Cmd) {
	cmd, isCmd := strings.CutPrefix(s, "/")
	var isShell bool
	if shell, isShell1 := strings.CutPrefix(s, "!"); isShell1 {
		cmd = shell
		isShell = isShell1
	}
	if !isShell && !isCmd {
		return *m, nil
	}
	args := strings.Fields(cmd)
	if isShell && args[0] == "cd" { // prevent misleading successful "!cd" exit code
		isShell = false
	}
	if isShell {
		c := exec.CommandContext(context.TODO(), "bash", "-c", cmd)
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
		content := fmt.Sprintf("Bash executed: %s\nExit Code %d\nOutput (in code fencing):\n\n```\n%s\n```\n", cmd, exitcode, output)
		m.addNewMessage("user", content)
		return m, nil

	}
	switch args[0] {
	default:
		return m.withAnyAux(fmt.Errorf("slash-command %q not found", args[0])), nil
	case "help":
		m.addNewMessage("", "# Help\n"+helpflags+"\n\n"+helpslash)
		return m, nil
	case "attach":
		m.addPendingImage(args[1])
		return m, nil
	case "approve", "deny":
		if len(args) != 2 {
			return m.WithError(fmt.Errorf("need ID to approve")), nil
		}
		id, err := uuid.Parse(args[1])
		if err != nil {
			return m.WithError(err), nil
		}
		// in a goroutine, send it back to the ui as an approval for notification
		return m, func() tea.Msg {
			if !doApproval("approve slash command", id, args[0] == "approve") {
				return fmt.Errorf("was not a known pending request")
			}
			return Approval{UUID: id, Approved: args[0] == "args", From: "slash"}
		}
	case "role": // used in [startGen]
		if len(args) == 1 {
			return m.WithTextMessage("Current role: " + global.role), nil
		}
		_, err := roles.ReadRole(args[1])
		if err != nil {
			return m.WithError(err), nil
		}
		global.role = args[1]
		return m.WithTextMessage("Current role: " + global.role), nil
	case "clear":
		m.convo = nil
		m.messages = nil
		m.renderConvo(true)
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
		*m = m.WithTextMessage(fmt.Sprintf("undid %d messages now have %d", n, len(m.convo)))
		*m = m.WithTextMessage(fmt.Sprintf("convo roles: %v", m.convo.TopN(10).Roles()))
		if m.ocfg.DB != nil {
			ctx := context.TODO()
			if _, err := m.ocfg.DB.Where("thread = ? AND created_at > ?", m.threadID, m.convo.Top().CreatedAt).Delete(ctx); err != nil {
				return m.WithError(err), nil
			}
		}
		return m, nil
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
