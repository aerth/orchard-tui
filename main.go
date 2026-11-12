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
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/aerth/mostly/godotenv"
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

var (
	version             = "0.0.x"
	gitcommit           = ""
	_         tea.Model = model{}
)

func randomPrompt() string {
	a := []string{
		"read and explain ./Makefile",
		"summarize README.md",
		"Hi",
		"what tools are available? give names and descriptions",
	}
	return a[rand.Intn(len(a))]
}

func main() {
	mllm.DefaultWriter = io.Discard
	wd, _ := os.Getwd()
	mllm.DefaultOptions.Debug = 0
	mllm.Flags()
	var dbfile string
	var smoketest bool
	flag.StringVar(&dbfile, "db", dbfile, "path to sqlite db file (optional, for persistence. in-memory usage: \"file::memory:?cache=shared\")")
	flag.StringVar(&wd, "change-dir", wd, "change directory before anything")
	//flag.BoolVar(&addbash, "bash", addbash, "add the default bash tool even when using toolbox tools")
	flag.BoolFunc("no-bash", "remove bash tool, only use toolboxes", func(s string) error {
		orchard.AllTools = nil
		return nil
	})
	flag.BoolVar(&smoketest, "smoke", smoketest, "say hello and exit (no tools)")
	flag.BoolFunc("version", "show version info", func(s string) error {
		fmt.Println(version)
		fmt.Println(gitcommit)
		os.Exit(1)
		return nil
	})
	flag.Parse()
	if cwd, _ := os.Getwd(); wd != "" && cwd != wd {
		if err := os.Chdir(wd); err != nil {
			log.Fatalf("chdir: %+v", err)
		}
	}
	if mllm.DefaultThread == uuid.Nil() {
		mllm.DefaultThread = mllm.ThreadHasher(wd) // use hash(currentdir) as thread uuid
	}
	_ = godotenv.Load(os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env"))
	if s := os.Getenv("ORCHARD_DB"); s != "" && dbfile == "" {
		dbfile = s
		dbfile = strings.Replace(dbfile, "~/", os.ExpandEnv("$HOME")+"/", 1)
		if _, err := os.Stat(dbfile); errors.Is(err, os.ErrNotExist) {
			_ = os.MkdirAll(filepath.Dir(dbfile), 0o750)
		}
		slog.Info("using ORCHARD_DB", "path", dbfile, "from", s)
	}
	tools := orchard.AllTools
	if dir := os.Getenv("ORCHARD_TOOLS"); dir != "" {
		dir = strings.Replace(dir, "~/", os.ExpandEnv("$HOME")+"/", 1)
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			fmt.Println("error loading toolbox:", err)
			return
		}
		tools = append(orchard.AllTools, box.AllTools()...)
	}
	if dbfile != "" {
		opendb(dbfile)
	}
	if smoketest {
		return
	}
	if err := main2(tools); err != nil {
		if !errors.Is(err, ErrConfig) {
			slog.Error("fatal", "err", err)
		} else {
			fmt.Println("Example: LLM_HOST='https://ollama.home.arpa/v1/chat/completions'  LLM_MODEL=gemma4:12b LLM_API_KEY=12345")
		}
		os.Exit(1)
	}
}

func getnames(tools mllm.Tools) []string {
	names := make([]string, len(tools))
	for i := range tools {
		names[i] = tools[i].GetName()
	}
	return names
}

var (
	ErrConfig  = errors.New("bad config")
	cancelchan = make(chan os.Signal, 10)
)

func main2(tools mllm.Tools) error {
	if global.gdb != nil {
		defer closedb()
	}
	ocfg := orchard.Config{
		E:            mllm.Default(),
		Think:        false,
		ThreadReader: nil,
		DB:           global.gdbx,
		Toolbox:      tools,
	}
	fmt.Println(welcomemodel{}.View().Content)
	if global.gdbx == nil {
		fmt.Println("Persistence is disabled (use -db flag for sqlite)")
	}
	fmt.Println()
	if err := startupcheck(os.Stdout); err != nil {
		fmt.Println("Fatal:", strings.ReplaceAll(err.Error(), "\n", " "))
		return ErrConfig
	}
	fmt.Println()
	toolnames := getnames(tools)
	fmt.Println("tools available:", toolnames)
	fmt.Println()
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
	s, _ := orchard.DefaultSystemPrompt(&orchard.PromptInput{})
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
	Stickybottom                          bool
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
	ta.Placeholder = "Send a message. Press Ctrl+Q to quit. Shift+Tab to toggle markdown."
	ta.SetVirtualCursor(false)
	ta.Focus() // activate input textarea

	ta.Prompt = "┃ "
	ta.CharLimit = 28000

	ta.SetWidth(30)
	ta.MinHeight = 3
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
		return nil
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
	if ta.Length() == 0 && len(convo) == 0 {
		ta.InsertString(randomPrompt())
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
		Stickybottom: true,
	}
	return x
}

func (m *model) renderConvo(full bool) {
	full = full || len(m.messages) == 0
	if len(m.convo) == 0 {
		return
	}
	if full {
		m.messages = renderConvoSlice(m.convo)
	} else {
		l := len(m.messages)
		add := len(m.convo) > l
		a := renderConvoSlice(m.convo[l-1:])
		if !add {
			copy(m.messages[l-1:], a[:])
		} else {
			m.messages = append(m.messages[:l-1], a...)
		}
	}
	m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
}

// Init see [model.View]
func (m model) Init() tea.Cmd {
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
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
		m.renderConvo(true)
		if m.Stickybottom {
			m.viewport.GotoBottom()
		}
	case os.Signal:
		slog.Info("signal received:", "sig", msg)
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
		if msg.IsEmpty() && msg.Aux == nil {
			slog.Info("added empty message to ui", "size_convo", len(m.convo), "size_rendered", len(m.messages), "role", msg.Role, "is_empty", msg.IsBlank())
			return m, nil
		}
		m.renderConvo(false)
		if m.Stickybottom {
			m.viewport.GotoBottom()
		}
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
		m.renderConvo(false)
		if m.Stickybottom {
			m.viewport.GotoBottom()
		}
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - m.textarea.Height())
		mdrend = tuilite.NewRenderer(msg.Width - 4)
		m.renderConvo(true)
		if m.Stickybottom {
			m.viewport.GotoBottom()
		}
	case tea.KeyPressMsg: // keybinds
		key := msg.String()
		switch key {
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
			m.renderConvo(true)
			if m.Stickybottom {
				m.viewport.GotoBottom()
			}
			return m, nil
		case "down", "pgdown", "end":
			if key == "end" {
				if m.Stickybottom {
					m.viewport.GotoBottom()
				}
			} else {
				m.viewport, _ = m.viewport.Update(msg)
			}
			m.Stickybottom = m.Stickybottom || m.viewport.AtBottom() || key == "end"
			return m, nil
		case "up", "pgup", "home":
			if key == "home" {
				m.viewport.GotoTop()
			} else {
				m.viewport, _ = m.viewport.Update(msg)
			}
			m.Stickybottom = false
			return m, nil

		case "enter":
			slog.Info("enter", "size_convo", len(m.convo), "convo", m.convo, "top", m.convo.Top().Aux)
			if !m.textarea.Focused() { // home page
				m.textarea.Focus()
				return m, nil
			}
			val := m.textarea.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}
			if val[0] == '/' || val[0] == '!' {
				m.textarea.Reset()
				return m.SlashCommandHandler(val)
			}
			m.textarea.Reset()
			usermsg := mllm.Message{Role: "user", Content: val}.
				ToThreadMessage(m.userName, m.threadID)
			m.convo = append(m.convo, *usermsg)
			m.renderConvo(false)
			if m.Stickybottom {
				m.viewport.GotoBottom()
			}
			{
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

	if e.SystemPrompt == nil {
		e.SystemPrompt = orchard.DefaultSystemPrompt
	}
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

	if len(e.Toolbox) == 0 {
		go global.SendFunc(warning("Using default tools..."))
		e.Toolbox = orchard.AllTools
	}
	e.Handlers = append(e.Handlers, pending.bghandle)

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

type warning string
