// Package tuilite easy now. Part of the Orchard AI project (https://github.com/aerth/orchard)
package tuilite

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

var ErrUserCtrlC = errors.New("ctrl+c pressed")

type MarkdownRenderer struct {
	TermRenderer *glamour.TermRenderer
	Alt          *glamour.TermRenderer
}

type TickMsg struct {
	TickTime time.Duration
	T        time.Time
	N        uint64
	D        time.Duration
}

type FakeMessage struct {
	Header  string
	Message string
}

func (f FakeMessage) String() string {
	if f.Header != "" {
		return fmt.Sprintf("## %s\n\n%s", f.Header, f.Message)
	}
	return f.Message
}

func SendMessage[T any](msg T) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}

const (
	StyleA      = 0
	StyleB      = 1
	StyleC      = 3
	StyleHidden = 4
)

func NewRenderer(width int) *MarkdownRenderer {
	if width == 0 {
		width = 120
	}
	width = max(30, width)
	innerWidth := max(80, width-40)
	horizontalBar := strings.Repeat("─", innerWidth)

	// Dynamically build the frames
	customStyle := styles.DarkStyleConfig
	customStyle.Emph.Faint = new(true)
	customStyle.Emph.Color = new("244")
	customStyle.Text.Color = new("14")
	customStyle.CodeBlock.BlockPrefix = "╭" + horizontalBar + "╮\n"
	customStyle.CodeBlock.BlockSuffix = "\n╰" + horizontalBar + "╯"
	customStyle.CodeBlock.IndentToken = new("│ ")
	customStyle.CodeBlock.Indent = new(uint(0))
	flags := []glamour.TermRendererOption{glamour.WithWordWrap(width), glamour.WithPreservedNewLines(), glamour.WithEmoji()}
	r, err := glamour.NewTermRenderer(append(flags, glamour.WithStyles(customStyle))...)
	if err != nil {
		slog.Error("could not make renderer", "err", err)
		return nil
	}
	alt, err := glamour.NewTermRenderer(append(flags, glamour.WithStyles(styles.DraculaStyleConfig))...)
	if err != nil {
		slog.Error("could not make alt renderer", "err", err)
		return nil
	}
	return &MarkdownRenderer{TermRenderer: r, Alt: alt}
}

func (tr *MarkdownRenderer) Render(in string, style int) (string, error) {
	if tr == nil {
		return in, nil
	}
	if style == StyleHidden {
		return "", nil
	}
	r := tr.TermRenderer
	if style != 0 && tr.Alt != nil {
		r = tr.Alt
	}
	b, err := r.RenderBytes([]byte(in))
	return string(b), err
}
