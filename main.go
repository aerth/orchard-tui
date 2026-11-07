package main

// A simple program demonstrating the text area component from the Bubbles
// component library.

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
	"uuid"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard-tui/pkg/tuilite"
	"github.com/aerth/orchard/pkg/toolboxes"
	"gorm.io/gorm"
)

type (
	NewAppendMessage mllm.ChatMessage
	Frag             struct {
		ID   uuid.UUID
		M    mllm.ChatMessage
		Done bool
	}
)

var global = struct {
	gdbx gorm.CreateInterface[mllm.ChatMessage] // for [orchard.Config.DB]
	gdb  *gorm.DB                               // for closer

	lastsent  time.Time     // to buffer before sending to UI
	SendFunc  func(tea.Msg) // [tea.Program.Send]
	cancelGen func()        // cancel latest gen, replaced by [startGen]
	Refresh   time.Duration
}{
	cancelGen: func() {},
	Refresh:   time.Second / 10,
}

var _ tea.Model = model{}

func main() {
	mllm.DefaultWriter = io.Discard
	mllm.DefaultThread = uuid.MustParse("00010100-6d4b-4e07-9703-b9baac13ffaa")
	mllm.DefaultOptions.Debug = 0
	mllm.Flags()
	var dbfile string
	flag.StringVar(&dbfile, "db", dbfile, "path to sqlite db file (optional, for persistence)")
	flag.Parse()
	if dbfile != "" {
		opendb(dbfile)
	}
	if err := main2(); err != nil {
		if !errors.Is(err, ErrConfig) {
			slog.Error("fatal", "err", err)
		} else {
			fmt.Println("Example: LLM_HOST='https://ollama.home.arpa/v1/chat/completions'  LLM_MODEL=gemma4:12b LLM_API_KEY=12345")
		}
		os.Exit(1)
	}
}

var (
	ErrConfig  = errors.New("bad config")
	cancelchan = make(chan os.Signal, 10)
)

func main2() error {
	if global.gdb != nil {
		defer closedb()
	}
	ocfg := orchard.Config{
		E:            mllm.Default(),
		Think:        false,
		ThreadReader: nil,
		DB:           global.gdbx,
		Toolbox:      orchard.AllTools,
	}
	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			return err
		}
		ocfg.Toolbox = box.AllTools()
	}
	fmt.Println(welcomemodel{}.View().Content)
	if global.gdbx == nil {
		fmt.Println("Persistence is disabled (use -db flag for sqlite)")
	}
	if err := startupcheck(os.Stdout); err != nil {
		fmt.Println("Fatal:", strings.ReplaceAll(err.Error(), "\n", " "))
		return ErrConfig
	}
	_, _ = os.Stdin.Read([]byte{0})

	if !mllm.Quiet {
		// log stuff
		logfilename := "/tmp/orchard.json.log"
		logfile, err := os.OpenFile(logfilename, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
		if err != nil {
			log.Printf("fatal: %v", err)
		}
		defer func() {
			_ = logfile.Close()
			println("closed log file", logfilename)
		}()
		slog.SetDefault(slog.New(slog.NewJSONHandler(logfile, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})))
		slog.Info("Hello Orchard")
	}
	p := tea.NewProgram(initialModel(ocfg), tea.WithoutSignalHandler())
	global.SendFunc = p.Send // does block
	signal.Notify(cancelchan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		x := 0
		var t1 time.Time
		for sig := range cancelchan {
			global.cancelGen()
			x++
			slog.Error("got signal", "also", len(cancelchan), "x", x)
			if time.Since(t1) > time.Second*10 {
				x = 1
			} else if x > 3 && time.Since(t1) < time.Second*10 {
				slog.Error("too many signals")
				global.SendFunc(tea.QuitMsg{})
			}
			t1 = time.Now()
			go global.SendFunc(sig) // still sending the signal for ui to cancel
		}
	}()
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func startupcheck(w io.Writer) error {
	ctx := context.TODO()
	s, _ := orchard.DefaultSystemPrompt()
	client := mllm.Default()
	client.StreamWriter.Writer = nil
	client.Writer = w
	_, err := client.Generate(ctx, s, "Create a short greeting for the user, as a robot from a different solar system. The user should be told to press ENTER to begin using the Orchard AI Agent system.", false)
	return err
}

type model struct {
	viewport viewport.Model
	messages []string
	convo    mllm.ChatMessages
	textarea textarea.Model
	page     int

	err      error
	ocfg     orchard.Config
	threadID uuid.UUID
	userName string // bob

	senderStyle, warningStyle, errorStyle lipgloss.Style
}

// View see [model.Update] [model.Init]
func (m model) View() tea.View {
	m.viewport.FillHeight = false
	viewportView := m.viewport.View()
	//slog.Info("viewport", "size", len(viewportView))
	v := tea.NewView(viewportView + "\n" + m.textarea.View())
	c := m.textarea.Cursor()
	if c != nil {
		c.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = c
	v.AltScreen = false
	v.WindowTitle = "Orchard AI"
	return v
}

func initialModel(ocfg orchard.Config) tea.Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.SetVirtualCursor(false)
	ta.Focus() // activate input textarea

	ta.Prompt = "┃ "
	ta.CharLimit = 28000

	ta.SetWidth(30)
	ta.SetHeight(3)
	ta.DynamicHeight = true

	// Remove cursor line styling
	s := ta.Styles()
	s.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(s)

	ta.ShowLineNumbers = true

	vp := viewport.New(viewport.WithWidth(30), viewport.WithHeight(10))
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)
	vp.SoftWrap = false

	ta.KeyMap.InsertNewline.SetEnabled(false)

	if mllm.DefaultThread == uuid.Nil() {
		slog.Error("nil thread ID")
	} else {
		slog.Info("welcome thread", "id", mllm.DefaultThread)
	}
	var convo mllm.ChatMessages

	// add previous convo to view
	if ocfg.DB != nil {
		got, err := ocfg.GormThreadReader(context.TODO(), ocfg.ThreadID)
		if err != nil {
			slog.Warn("thread reader", "err", err)
		}
		got = slices.DeleteFunc(got, func(v mllm.ChatMessage) bool {
			return v.IsBlank() && v.Aux == nil
		})
		if len(got) != 0 {
			convo = got
		}
		if top := convo.Top(); top.Role == "user" {
			ta.InsertString(top.Content)
			convo = convo[:len(convo)-1]
		}
	}
	if ta.Length() == 0 {
		ta.InsertString("read and explain ./Makefile")
	}
	x := model{
		textarea:     ta,
		viewport:     vp,
		senderStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		warningStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		err:          nil,
		userName:     "Bob",
		threadID:     mllm.DefaultThread,
		ocfg:         ocfg,
		messages:     renderConvoSlice(convo),
		convo:        convo,
	}
	return x
}

// Init see [model.View]
func (m model) Init() tea.Cmd {
	println("INIT")
	m.messages = renderConvoSlice(m.convo)
	m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
	m.viewport.GotoBottom()
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	default:
		typ := fmt.Sprintf("%T", msg)
		if !strings.HasPrefix(typ, "tea.") && !strings.HasPrefix(typ, "cursor.") {
			slog.Error(fmt.Sprintf("unknown packet %s %#v", typ, msg))
		}
	case struct{}:
		i := 0
		if l := len(m.convo); l > 20 {
			i = l - 20
		}
		m.messages = append(m.messages[i:], renderConvoSlice(m.convo[i:])...) // TODO: optimize again
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
		slog.Info("is finished")
	case os.Signal:
		global.cancelGen()
		return m, nil
	case string:
		return m.WithMessage(msg), nil
	case error:
		return m.WithError(msg), nil
	case tea.PasteStartMsg:
		return m, nil
	case tea.PasteMsg:
		m.textarea.InsertString(msg.Content)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	case tea.PasteEndMsg:
		return m, nil
	case NewAppendMessage: // toolouts, or blank assistant
		m.convo = append(m.convo, mllm.ChatMessage(msg))
		m.messages = renderConvoSlice(m.convo) // TODO: optimize again
		if msg.IsEmpty() && msg.Aux == nil {
			slog.Info("added empty message to ui", "size_convo", len(m.convo), "size_rendered", len(m.messages), "role", msg.Role, "is_empty", msg.IsBlank())
			return m, nil
		}
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
		return m, nil
	case Frag: // incoming assistant stream, fast
		if msg.M.IsBlank() {
			slog.Error("got empty frag", "m", msg.M)
			return m, nil
			//		} else if !msg.Done {
			//			slog.Info("got frag ", "atID", msg.ID, "finished", msg.Done, "size", len(m.convo), "content", msg.M.Content, "toolcalls", len(msg.M.ToolCalls))
		}
		//l := len(m.convo)
		at := m.convo.IndexOf(msg.ID)
		if at < 0 {
			slog.Error("frag convo", "convo", len(m.convo), "at", at, "want", msg.ID, "top", m.convo.Top().MessageID)
			return m, nil
		}
		m.convo[at] = msg.M //m.convo[at].Add(msg.M)
		m.convo[at].CreatedAt = time.Now()
		m.messages = renderConvoSlice(m.convo) // TODO: optimize again
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.textarea.SetWidth(msg.Width)
		mdrend = tuilite.NewRenderer(msg.Width - 4)
		m.viewport.SetHeight(msg.Height - m.textarea.Height())

		if len(m.messages) > 0 {
			// Wrap content before setting it.
			m.messages = renderConvoSlice(m.convo) // TODO: optimize again
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		}
		m.viewport.GotoBottom()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			cancelchan <- syscall.SIGINT
			return m.WithMessage("Cancelled (press a few more times to quit)"), nil
		case "ctrl+q":
			global.cancelGen()
			// fmt.Println(m.textarea.Value()) // if [tea.View.AltScreen] is on
			return m, tea.Quit
		case "shift+enter":
			m.textarea.InsertRune('\n')
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		case "shift+tab":
			if mdrend != nil {
				mdrend = nil
			} else {
				mdrend = tuilite.NewRenderer(m.viewport.Width())
			}
			m.messages = renderConvoSlice(m.convo) // TODO: optimize again
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
			m.viewport.GotoBottom()
			return m, nil
		case "enter":
			slog.Info("enter", "size_convo", len(m.convo), "convo", m.convo, "top", m.convo.Top().Aux)
			if !m.textarea.Focused() { // home page
				m.textarea.Focus()
				return m, nil
			}
			global.cancelGen()
			val := m.textarea.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}
			usermsg := mllm.Message{Role: "user", Content: val}.
				ToThreadMessage(m.userName, m.threadID)
			m.convo = append(m.convo, *usermsg)
			m.messages = renderConvoSlice(m.convo) // TODO: optimize again
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
			m.textarea.Reset()
			m.viewport.GotoBottom()
			{
				global.cancelGen()
				go startGen(context.TODO(), m.ocfg, slices.DeleteFunc(slices.Clone(m.convo), RemoveEmpty), &PendingStreamTUI{
					From:   m.ocfg.From,
					Thread: m.threadID,
				})
			}
			return m, nil
		default:
			if !m.textarea.Focused() { // home page
				m.textarea.Focus()
				return m, nil
			}
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

func startGen(ctx context.Context, e orchard.Config, actualconvo mllm.ChatMessages, pending *PendingStreamTUI) {
	global.cancelGen() // one gen at a time
	if len(actualconvo) == 0 || actualconvo[0].Role == "" {
		slog.Error("cant stream empty convo", "size", len(actualconvo), "messages", actualconvo)
		global.SendFunc(fmt.Errorf("cant stream empty convo, size=%d", len(actualconvo)))
		return
	}
	ctx, global.cancelGen = context.WithCancel(ctx)
	e.SystemPrompt = orchard.DefaultSystemPrompt
	slog.Info("starting", "size_convo", len(actualconvo), "top", actualconvo.Top(), "db", e.DB != nil)
	if e.DB == nil {
		e.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
			return actualconvo, nil
		}
	} else {
		if err := e.DB.Create(ctx, &actualconvo[len(actualconvo)-1]); err != nil {
			go global.SendFunc(err)
			return
		}
	}

	// add our [mllm.StreamHandler] to get streamed text to UI
	e.Handlers = append(e.Handlers, pending.bghandle)
	if len(e.Toolbox) == 0 {
		e.Toolbox = orchard.AllTools
	}

	// orchard hooks
	e.PreTextHook = pending.PreText
	e.NonTextHook = pending.NonText
	e.FinishHook = pending.Finish

	// orchard ai - multiple rounds, tool calls
	out, err := orchard.Create(ctx, e.ThreadID, e)
	if err != nil {
		slog.Error("error streaming", "err", err)
		go global.SendFunc(err)
		return
	}
	_ = out
	pending.reset()
	slog.Info("got out", "n", len(out))
}
