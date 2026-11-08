package main

import (
	"fmt"
	"log/slog"
	"strings"
	"uuid"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard-tui/pkg/tuilite"
)

func renderConvoSlice(convo mllm.ChatMessages) []string {
	s := make([]string, len(convo))
	for i := range convo {
		if convo[i].Aux != nil {
			s[i] = fmt.Sprint(convo[i].Aux)
		} else {
			s[i] = FormatMessage(convo[i])
		}
	}
	return s
}

var mdrend = tuilite.NewRenderer(80)

func FormatMessage(cm mllm.ChatMessage) string {
	x := formatMessage(cm)
	y, z := mdrend.Render(x, 0)
	if z != nil {
		slog.Error("markdown", "err", z)
		return x
	}
	return y
}

func formatMessage(cm mllm.ChatMessage) string {
	if cm.Aux != nil {
		return fmt.Sprint(cm.Aux)
	}
	if cm.IsEmpty() {
		return ""
	}
	if cm.Role != "" && cm.Role != "assistant" {
		return strings.Title(cm.Role) + ": " + strings.TrimSpace(cm.Content)
	}
	if cm.Role == "" {
		cm.Role = "???"
	}
	if strings.TrimSpace(cm.Content) == "" && len(cm.ToolCalls) == 0 {
		cm.Content = "[pending]"
	}
	if len(cm.ToolCalls) == 0 {
		return "Bot: " + cm.Content
	}
	var buf strings.Builder

	buf.WriteString("Bot: ")
	buf.WriteString(cm.Content)
	buf.WriteString("\n---\n")
	for i := range cm.ToolCalls {
		buf.WriteString("tool call: ")
		buf.WriteString(string(cm.ToolCalls[i])) // is js
		buf.WriteString("\n---\n")
	}
	return buf.String()
}

func NewAux(s string, threadID uuid.UUID) *mllm.ChatMessage {
	latest := mllm.Message{}.ToThreadMessage("", threadID)
	latest.Aux = s
	return latest
}

func (m model) WithError(v error) model {
	latest := m.errorStyle.Render("Error:") + " " + v.Error()
	return m.WithMessage(latest)
}

func (m model) WithMessage(v string) model {
	latest := mllm.Message{}.ToThreadMessage("", m.threadID)
	latest.Aux = v
	m.convo = append(m.convo, *latest)
	m.renderConvo(false)
	m.viewport.GotoBottom()
	return m
}

func RemoveEmpty(v mllm.ChatMessage) bool {
	return v.IsEmpty()
}
