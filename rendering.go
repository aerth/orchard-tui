package main

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode"
	"uuid"

	"charm.land/lipgloss/v2"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard-tui/pkg/tuilite"
)

func renderConvoSlice(convo mllm.ChatMessages, renderfunc FormatterFunc) []string {
	s := make([]string, len(convo))
	for i := range convo {
		// if convo[i].Aux != nil {
		//		s[i] = fmt.Sprint(convo[i].Aux)
		//		} else {
		s[i] = FormatMessage(convo[i])
		//		}
	}
	return s
}

var mdrend = tuilite.NewRenderer(80)

type (
	FormatterFuncSwitch = func(m mllm.ChatMessage) string
	FormatterFunc       = func(m mllm.ChatMessage) string
)

func defaultFormatterSwitch(m mllm.ChatMessage) string {
	if mdrend == nil {
		return FormatMessageRaw(m)
	}
	return FormatMessage(m)
}

func FormatMessage(cm mllm.ChatMessage) string {
	x := FormatMessageRaw(cm)
	if mdrend == nil || alreadyTermRendered(cm.Aux) {
		return x
	}
	y, z := mdrend.Render(x, tuilite.StyleB)
	if z != nil {
		slog.Error("markdown", "err", z)
		return x
	}
	return y
}

var styles = struct {
	senderStyle, warningStyle, errorStyle lipgloss.Style
}{
	senderStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
	errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	warningStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
}

func alreadyTermRendered(v any) bool {
	switch v.(type) {
	case error:
		return true
	default:
		return false
	}
}

func formatAux(v any) string {
	switch aux := v.(type) {
	case error:
		latest := styles.errorStyle.Render("Error:") + " " + aux.Error()
		return latest
	case string:
		return aux
	default:
		return fmt.Sprint(aux)
	}
}

func FormatMessageRaw(cm mllm.ChatMessage) string {
	if cm.Aux != nil {
		return formatAux(cm.Aux)
	}
	if cm.IsEmpty() {
		return ""
	}
	if cm.Role != "" && cm.Role != "assistant" {
		role := []rune(cm.Role)
		role[0] = unicode.ToTitle(role[0])
		return string(role) + ": " + strings.TrimSpace(cm.Content) //nolint: deprecated
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
		buf.WriteString("tool call:\n```json\n")
		buf.WriteString(string(cm.ToolCalls[i])) // is js, no newline
		buf.WriteString("\n```\n---\n")
	}
	return buf.String()
}

func NewAux(s string, threadID uuid.UUID) *mllm.ChatMessage {
	latest := mllm.Message{}.ToThreadMessage("", threadID)
	latest.Aux = s
	return latest
}

func (m model) WithError(v error) model {
	return m.withAnyAux(v)
}

func (m model) WithTextMessage(v string) model {
	return m.withAnyAux(v)
}

func (m model) withAnyAux(v any) model {
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
