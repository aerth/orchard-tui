package main

// A simple program demonstrating the text area component from the Bubbles
// component library.

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
	"uuid"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
)

var cancelGen = func() {}

var actualconvo mllm.ChatMessages

func main() {
	ocfg := orchard.Config{
		E:     mllm.Default(),
		Think: false,
		ThreadReader: func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
			return actualconvo, nil
		},
		Toolbox: nil,
	}
	p := tea.NewProgram(initialModel(ocfg))
	sender = p.Send
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Oof: %v\n", err)
	}
}

type model struct {
	viewport    viewport.Model
	messages    []string
	convo       mllm.ChatMessages
	textarea    textarea.Model
	senderStyle lipgloss.Style
	err         error
	ocfg        orchard.Config
	threadID    uuid.UUID
	userName    string // bob
}

func initialModel(ocfg orchard.Config) model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.SetVirtualCursor(false)
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	// Remove cursor line styling
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(s)

	ta.ShowLineNumbers = false

	vp := viewport.New(viewport.WithWidth(30), viewport.WithHeight(5))
	vp.SetContent(`Welcome to the chat room!
Type a message and press Enter to send.`)
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return model{
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
		userName:    "Bob",
		threadID:    uuid.Nil(),
		ocfg:        ocfg,
		convo:       mllm.ChatMessages{},
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

var (
	pendingMessage  mllm.ChatMessage
	pendingResponse mllm.ChatResponse // actual
	pendingAtIndex  = -1
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case struct{}:
		println("GOT PDATE")
		if pendingAtIndex <= 0 {
			return m, nil
		}
		m.convo[pendingAtIndex] = pendingMessage
		latest := m.senderStyle.Render("AI: ") + pendingMessage.Content
		m.messages[pendingAtIndex] = latest
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - m.textarea.Height())

		if len(m.messages) > 0 {
			// Wrap content before setting it.
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		}
		m.viewport.GotoBottom()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			cancelGen()
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case "enter":
			cancelGen()
			val := m.textarea.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}
			m.convo = append(m.convo, mllm.NewChatMessage(mllm.Message{Role: "user", Content: val}, "User", m.threadID))
			pendingAtIndex = len(m.convo)
			m.convo = append(m.convo, mllm.NewChatMessage(mllm.Message{Role: "assistant", Content: ""}, m.ocfg.From, m.threadID))
			m.messages = append(m.messages, m.senderStyle.Render("You: ")+val)
			m.messages = append(m.messages, m.senderStyle.Render("Bot: "))
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
			m.textarea.Reset()
			m.viewport.GotoBottom()
			ctx := context.TODO()
			go startGen(ctx, m.ocfg, slices.Clone(m.convo), bghandle)
			return m, nil
		default:
			// Send all other keypresses to the textarea.
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		}

	case cursor.BlinkMsg:
		// Textarea should also process cursor blinks.
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

var (
	lastsent time.Time
	sender   func(tea.Msg)
)

func bghandle(cr mllm.ChatResponse) error {
	if err := pendingResponse.Handler(cr); err != nil {
		pendingResponse.Message.Content += "..."
		pendingResponse.Done = true
		pendingMessage = pendingResponse.Message.ToChatMessage("Bot")
		sender(struct{}{})
		return err
	}
	if cr.Done || time.Since(lastsent) > time.Second/4 {
		lastsent = time.Now()
		pendingMessage = pendingResponse.Message.ToChatMessage("Bot")
		sender(struct{}{})
	}
	return nil
}

type TemplateData struct {
	WorkingDirectory string
	CurrentTime      string
}

func executeTemplateString(s string, d TemplateData) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(s))
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func startGen(ctx context.Context, e orchard.Config, convo mllm.ChatMessages, handler mllm.StreamHandler) {
	if len(convo) == 0 || convo[0].Role == "" {
		slog.Error("cant stream empty convo", "size", len(convo))
		return
	}
	e.SystemPrompt = func() (string, error) {
		return "You are an AI Agent named Orchard. The user is a superuser.", nil
	}
	actualconvo = convo
	pendingMessage = mllm.ChatMessage{}
	pendingResponse = mllm.ChatResponse{}
	e.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return convo[:pendingAtIndex], nil
	}
	e.Options.Debug = 2
	e.Handlers = append(e.Handlers, handler)
	if len(e.Toolbox) == 0 {
		e.Toolbox = orchard.AllTools
	}
	out, err := orchard.Create(ctx, e.ThreadID, e)
	//err := <-e.StreamBG(ctx, convo.ToMessages(), e.Think, handler)
	if err != nil {
		slog.Error("error streaming", "err", err)
		return
	}
	slog.Info("got out", "n", out)
}

func (m model) View() tea.View {
	viewportView := m.viewport.View()
	v := tea.NewView(viewportView + "\n" + m.textarea.View())
	c := m.textarea.Cursor()
	if c != nil {
		c.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = c
	v.AltScreen = true
	return v
}

var _ tea.Model = model{}
