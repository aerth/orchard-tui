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
	"github.com/aerth/mostly/ncode"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard-tui/pkg/roles"
	"github.com/aerth/orchard-tui/pkg/tuilite"
	"github.com/aerth/orchard/pkg/builtintools"
	"github.com/aerth/orchard/pkg/toolboxes"
	"github.com/aerth/orchard/pkg/visions"
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

var programOptions = []tea.ProgramOption{
	tea.WithoutSignalHandler(),
}

var global = struct {
	gdbx gorm.CreateInterface[mllm.ChatMessage] // for [orchard.Config.DB]
	gdb  *gorm.DB                               // for closer

	lastsent  time.Time     // to buffer before sending to UI
	SendFunc  func(tea.Msg) // [tea.Program.Send]
	cancelGen func()        // cancel latest gen, replaced by [startGen]
	Refresh   time.Duration
	envFile   string // cmdline flag, may be 'none'
	role      string // to read ./roles/match*.md
	orchard.Config
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

func tilde(s string) string {
	s = strings.Replace(s, "~/", os.ExpandEnv("$HOME")+"/", 1)
	return os.ExpandEnv(s)
}

func toolboxesLoad(dirsfull string) (mllm.Tools, error) {
	tools := orchard.AllTools
	dirs := strings.SplitSeq(dirsfull, ":")

	for dir := range dirs {
		dir = strings.TrimSpace(dir)
		if len(dir) == 0 {
			continue
		}
		dir = tilde(dir)
		box, err := toolboxes.NewToolboxFS(dir)
		if err != nil {
			return tools, stackerr.Errorf("error loading toolbox %q: %w", dir, err)
		}
		tools = append(tools, box.AllTools()...)
	}
	return slices.DeleteFunc(tools, IsNotAllowedTool), nil
}

var sloglvl = new(slog.LevelVar)

// see [global]
func Flags(dbfile, wd *string, smoketest *bool) {
	flag.StringVar(&global.envFile, "e", global.envFile, "use a different env file (or 'none') instead of ~/.config/orchard/env")
	mllm.Flags()
	orchard.Flags(sloglvl)

	if dbfile != nil {
		flag.StringVar(dbfile, "db", *dbfile, "path to sqlite db file (optional, for persistence. in-memory usage: \"file::memory:?cache=shared\")")
	}
	if wd != nil {
		flag.StringVar(wd, "change-dir", *wd, "change directory before anything")
	}
	//flag.BoolVar(&addbash, "bash", addbash, "add the default bash tool even when using toolbox tools")
	flag.Func("guardian-allow", "additional text to help bash guardian identify allowed commands, may be used multiple times", func(s string) error {
		builtintools.AddGuardPrompt(s)
		return nil
	})
	flag.BoolFunc("no-bash", "remove bash tool, only use toolboxes", func(s string) error {
		orchard.AllTools = nil
		return nil
	})
	if smoketest != nil {
		flag.BoolVar(smoketest, "smoke", *smoketest, "say hello, list tools available, and exit (no tool calling)")
	}
	flag.BoolFunc("version", "show version info", func(s string) error {
		fmt.Println(version)
		fmt.Println(gitcommit)
		os.Exit(1)
		return nil
	})
	flag.Func("tool", "add one or comma/space-separated list of tools to be allowed. may be used multiple times.", func(toolName string) error {
		toolName = strings.ReplaceAll(toolName, ",", " ")
		approvals.allowedToolNames = append(approvals.allowedToolNames, strings.Fields(toolName)...)
		return nil
	})
	flag.Func("require-confirm", "add one or comma/space-separated list of tools to be allowed but confirmation is needed. may be used multiple times.", func(toolName string) error {
		toolName = strings.ReplaceAll(toolName, ",", " ")
		approvals.allowedToolNames = append(approvals.allowedToolNames, strings.Fields(toolName)...)
		approvals.needsApprovals = append(approvals.needsApprovals, strings.Fields(toolName)...)
		return nil
	})
}

func main() {
	mllm.DefaultWriter = io.Discard
	wd, _ := os.Getwd()
	mllm.DefaultOptions.Debug = 0
	sloglvl.Set(slog.LevelDebug)
	var dbfile string
	var smoketest bool
	Flags(&dbfile, &wd, &smoketest)
	flag.Parse()
	// load missing env
	if global.envFile == "" {
		envfile := os.ExpandEnv("${XDG_CONFIG_HOME}/orchard/env")
		if _, err := os.Stat(envfile); err != nil {
			if x := os.ExpandEnv("${HOME}/.config/orchard/env"); x != envfile {
				envfile = x
				if _, err = os.Stat(envfile); err != nil {
					envfile = ""
				}
			}
		}
		global.envFile = envfile
	}
	if global.envFile != "" && global.envFile != "none" {
		if err := godotenv.Load(global.envFile); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Fatalf("dot env: %+v", err)
			}
		}
		global.envFile = "" // :)
	}
	if cwd, _ := os.Getwd(); wd != "" && cwd != wd {
		if err := os.Chdir(wd); err != nil {
			log.Fatalf("chdir: %+v", err)
		}
	}
	if mllm.DefaultThread == uuid.Nil() {
		mllm.DefaultThread = mllm.ThreadHasher(wd) // use hash(currentdir) as thread uuid
	}
	if s := os.Getenv("ORCHARD_DB"); s != "" && dbfile == "" {
		dbfile = tilde(s)
		if _, err := os.Stat(dbfile); errors.Is(err, os.ErrNotExist) {
			_ = os.MkdirAll(filepath.Dir(dbfile), 0o750)
		}
		slog.Info("using ORCHARD_DB", "path", dbfile, "from", s)
	}
	var tools mllm.Tools
	{

		dirs := os.Getenv("ORCHARD_TOOLS")
		var err error
		if tools, err = toolboxesLoad(dirs); err != nil {
			slog.Error("load toolboxes", "err", err)
			return
		}

		if dirs == "" && (len(approvals.allowedToolNames) == 0 || (!slices.Contains(approvals.allowedToolNames, "run_bash"))) {
			println("no toolboxes configured, shell confirmation will be required for this session. to change this, add explicit flag: --tool run_bash to proceed without confirmations.")
			time.Sleep(time.Second)
			// no script tools. assume first-time user and enable confirmation for run_bash
			// test with -e none
			approvals.needsApprovals = append(approvals.needsApprovals, "run_bash")
		}

	}
	tools = slices.DeleteFunc(tools, IsNotAllowedTool)
	if dbfile != "" {
		opendb(dbfile)
	}
	if err := main2(tools, smoketest); err != nil {
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

func main2(tools mllm.Tools, smoketest bool) error {
	ocfg := orchard.Config{
		E:            mllm.Default(),
		Think:        false,
		ThreadReader: nil,
		DB:           global.gdbx,
		Toolbox:      tools,
		PromptSuffix: "We are in a TUI that renders markdown nicely for the user.",
	}
	fmt.Println(welcomemodel{}.View().Content)
	if global.gdbx == nil {
		fmt.Println("Persistence is disabled (use -db flag for sqlite)")
	}
	fmt.Println()
	if err := startupcheck(ocfg, os.Stdout); err != nil {
		fmt.Println("Fatal:", strings.ReplaceAll(err.Error(), "\n", " "))
		closedb()
		return ErrConfig
	}
	fmt.Println()
	toolnames := getnames(tools)
	fmt.Println("tools available:", toolnames)
	if x := approvals.needsApprovals; len(x) != 0 {
		fmt.Println("tools requiring confirmation:", x)
	}
	if len(tools) != len(toolnames) {
		slog.Error("weird", "n", len(tools), "n2", len(toolnames))
		return ErrConfig
	}
	if len(tools) == 0 {
		println("no tools found")
		return ErrConfig
	}
	if !smoketest && !mllm.Quiet {
		// log stuff
		logfilename := "/tmp/orchard.json.log"
		logfile, err := os.OpenFile(logfilename, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
		if err != nil {
			log.Printf("fatal: %v", err)
		}
		defer func() {
			slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
				AddSource: true,
				Level:     sloglvl,
			})))
			closedb()
			if err := logfile.Close(); err != nil {
				println("closing log file", "err", err.Error(), "name", logfilename)
			} else {
				println("closed log file", logfilename)
			}
		}()
		slog.SetDefault(slog.New(slog.NewJSONHandler(logfile, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})))
		slog.Info("Hello Orchard")
	} else {
		defer closedb()
	}
	p := tea.NewProgram(initialModel(ocfg), programOptions...)
	if p == nil {
		return fmt.Errorf("could not make tea program")
	}
	global.SendFunc = p.Send // does block
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
	if smoketest {
		return nil
	}
	_, _ = os.Stdin.Read([]byte{0})
	signal.Notify(cancelchan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func startupcheck(client orchard.Config, w io.Writer) error {
	ctx := context.TODO()
	//client := orchard.Config{E: mllm.Default()}
	client.Toolbox = nil
	client.StreamWriter.Writer = nil
	client.Writer = w
	s, err := client.SystemPrompt()
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	if len(s) == 0 {
		return stackerr.Errorf("no prompt template")
	}
	_, err = client.Generate(ctx, s,
		"Create a (very) short greeting for the user, as a robot from a different solar system. "+
			"In it, the user should be told to press the ENTER key "+
			"to begin using the Orchard AI Agent system. Markdown OK. No filler text, your output will be used directly.", false)
	if err != nil {
		return stackerr.Errorf("%w", err)
	}
	return nil
}

type model struct {
	viewport viewport.Model
	messages []string
	convo    mllm.ChatMessages // convo is messages. but some are only msg.Aux and no msg.Content,msg.Role.
	textarea textarea.Model
	page     int

	err      error
	ocfg     orchard.Config
	threadID uuid.UUID
	userName string // bob

	Stickybottom bool

	// state

	pendingAttachImages []string
	mostRecentRequest   mllm.ChatRequest
	mostRecentResponse  mllm.ChatResponse
}

// View see [model.Update] [model.Init]
func (m model) View() tea.View {
	m.viewport.FillHeight = false
	viewportView := m.viewport.View()
	//slog.Info("viewport", "size", len(viewportView))
	var v tea.View
	switch m.page {
	case 2:
		v.Content = viewportView
	default:
		v.Content = viewportView + "\n" + m.textarea.View()
	}
	c := m.textarea.Cursor()
	if c != nil {
		c.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = c
	v.AltScreen = false
	v.WindowTitle = "Orchard AI"
	v.MouseMode = tea.MouseModeCellMotion
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
	ocfg.PreRequest = func(request *mllm.ChatRequest) error {
		if request == nil {
			return nil
		}
		var req mllm.ChatRequest = *request
		req.Messages = slices.Clone(req.Messages)
		// TODO copy ChatRequest
		go global.SendFunc(req) // for the "most recent request" view
		return nil
	}

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
		topTime := convo.Top().CreatedAt
		if top := convo.Top(); top.Role == "user" {
			ta.InsertString(top.Content)
			convo = convo[:len(convo)-1]
		}
		if len(convo) != 0 {
			convo = append(convo, *mllm.Message{Role: "user", Content: fmt.Sprintf("The conversation has resumed after %s since the most recent message.", time.Since(topTime).Truncate(time.Second))}.ToThreadMessage("", ocfg.ThreadID))
		}
	}
	if ta.Length() == 0 && len(convo) == 0 {
		ta.InsertString(randomPrompt())
	}
	x := model{
		textarea:     ta,
		viewport:     vp,
		err:          nil,
		userName:     "Bob",
		page:         1, // home page
		threadID:     mllm.DefaultThread,
		ocfg:         ocfg,
		messages:     renderConvoSlice(convo, FormatMessageRaw),
		convo:        convo,
		Stickybottom: true,
	}
	return x
}

var pages = []string{
	// 0: "home",
	1: "chat",
	2: "recent request",
	3: "recent response",
}

func (m *model) renderConvo(full bool) {
	full = full || len(m.messages) == 0
	if len(m.convo) == 0 {
		return
	}
	rfn := defaultFormatterSwitch
	if full {
		m.messages = renderConvoSlice(m.convo, rfn)
	} else {
		l := len(m.messages)
		// Guard against an empty rendered buffer (e.g. right after "/clear"),
		// where indexing into the old slice would underflow.
		if l == 0 {
			l = len(m.convo)
		}

		add := len(m.convo) > l // convo always is bigger or equal to messages
		if !add {
			a := renderConvoSlice(m.convo[l:], rfn)
			copy(m.messages[l:], a[:])
		} else {
			a := renderConvoSlice(m.convo[l:], rfn)
			m.messages = append(m.messages[l:], a...)
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
	case mllm.ChatResponse: // for debug page
		m.mostRecentResponse = msg
		return m, nil
	case mllm.ChatRequest: // for debug page
		m.mostRecentRequest = msg
		return m, nil
	case string:
		return m.WithTextMessage(msg), nil
	case error:
		return m.WithError(msg), nil
	case tea.PasteStartMsg:
		return m, nil
	case tea.PasteMsg:
		m.textarea.InsertString(msg.Content)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(struct{}{})
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
		if mdrend != nil {
			mdrend = tuilite.NewRenderer(msg.Width - 4)
		}
		m.renderConvo(true)
		if m.Stickybottom {
			m.viewport.GotoBottom()
		}
	case tea.KeyPressMsg: // keybinds
		key := msg.String()
		switch key {
		case "ctrl+1":
			m.page = 1
			m.syncpage()
			return m, nil
		case "ctrl+2":
			m.page = 2
			m.syncpage()
			return m, nil
		case "ctrl+3":
			m.page = 3
			m.syncpage()
		case "ctrl+c", "esc":
			cancelchan <- syscall.SIGINT
			return m.WithTextMessage("Cancelled (press a few more times to quit)"), nil
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
			m.syncpage()
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
			if val[0] == '/' || val[0] == '!' || isCommandWord(val) {
				m.textarea.Reset()
				return m.SlashCommandHandler(val)
			}
			m.textarea.Reset()
			usermsg := mllm.Message{Role: "user", Content: val}.
				ToThreadMessage(m.userName, m.threadID)
			if err := visions.AttachImages(val, &usermsg.Message); err != nil {
				return m.WithError(err), nil
			}
			if len(m.pendingAttachImages) != 0 {
				if err := visions.AttachImagesPaths(m.pendingAttachImages, &usermsg.Message); err != nil {
					return m.WithError(err), nil
				}
				m.pendingAttachImages = nil
			}
			m.convo = append(m.convo, *usermsg)
			m.renderConvo(false)
			if m.Stickybottom {
				m.viewport.GotoBottom()
			}
			{
				go startGen(context.TODO(), m.ocfg,
					slices.DeleteFunc(slices.Clone(m.convo), RemoveEmpty),
					&PendingStreamTUI{
						From:   m.ocfg.From,
						Thread: m.threadID,
					})
			}
			return m, nil
		default:
			if s := msg.String(); strings.Count(s, "+") != 0 {
				slog.Info("weird button", "m", msg.String())
			}
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

func isCommandWord(val string) bool {
	for arg := range strings.FieldsSeq(val) {
		switch arg {
		case "help", "exit":
			return true
		}
	}
	return false
}

func (m *model) gotoPage1() {
	m.renderConvo(true)
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
}

func (m *model) syncpage() {
	switch m.page {
	case 1:
		m.gotoPage1()
	case 2:
		m.gotoPage2()
	case 3:
		m.gotoPage3()
	}
}

func (m *model) gotoPage2() {
	js := string(ncode.JSONIndent(m.mostRecentRequest))
	md := "```json\n" + js + "\n```\n"
	md += fmt.Sprintf("\n(%d messages)\n", len(m.mostRecentRequest.Messages))
	md += fmt.Sprintf("\n(%d tokens)\n", m.mostRecentResponse.Stats.TotalTokens)

	r, err := mdrend.Render(md, tuilite.StyleB)
	if err != nil {
		slog.Error("rend", "err", err)
	} else {
		md = r
	}
	m.viewport.SetContent(md)
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
}

func (m *model) gotoPage3() {
	js := string(ncode.JSONIndent(m.mostRecentResponse))
	md := "```json\n" + js + "\n```\n"
	md += fmt.Sprintf("\n(%d chars)\n", len(m.mostRecentResponse.Message.Content))
	md += fmt.Sprintf("\n(%d tokens)\n", m.mostRecentResponse.Stats.TotalTokens)

	r, err := mdrend.Render(md, tuilite.StyleB)
	if err != nil {
		slog.Error("rend", "err", err)
	} else {
		md = r
	}
	m.viewport.SetContent(md)
	if m.Stickybottom {
		m.viewport.GotoBottom()
	}
}

func startGen(ctx context.Context, e orchard.Config, actualconvo mllm.ChatMessages, pending *PendingStreamTUI) {
	global.cancelGen() // one gen at a time
	if len(actualconvo) == 0 || actualconvo[0].Role == "" {
		slog.Error("cant stream empty convo", "size", len(actualconvo), "messages", actualconvo)
		global.SendFunc(fmt.Errorf("cant stream empty convo, size=%d", len(actualconvo)))
		return
	}
	ctx, global.cancelGen = context.WithCancel(ctx)
	slog.Info("starting", "size_convo", len(actualconvo), "top", actualconvo.Top().Message, "db", e.DB != nil)
	e.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
		return actualconvo, nil
	}
	if e.DB != nil {
		if err := e.DB.Create(ctx, &actualconvo[len(actualconvo)-1]); err != nil {
			go global.SendFunc(err)
			return
		}
	}

	// add our [mllm.StreamHandler] to get streamed text to UI

	var collector mllm.ChatResponse
	e.Handlers = append(e.Handlers, pending.bghandle, func(cr mllm.ChatResponse) error {
		if collector.Done {
			collector = mllm.ChatResponse{}
		}
		if err := collector.Handler(cr); err != nil {
			return err
		}
		if cr.Done {
			global.SendFunc(collector)
		}
		return nil
	})

	// orchard hooks
	e.PreTextHook = pending.PreText
	e.PreToolHook = func(ctx context.Context, name string, args map[string]any) error {
		if len(approvals.allowedToolNames) != 0 && !slices.Contains(approvals.allowedToolNames, name) {
			return fmt.Errorf("tool never existed")
		}
		if slices.Contains(approvals.needsApprovals, name) {
			req := createRequest("Tool run: " + name)
			go global.SendFunc("You must `/approve` or `/deny` the request:\n" + req.UUID.String())
		waiting:
			for !isResolved(req.UUID) {
				select {
				case <-ctx.Done():
					return context.Cause(ctx)
				case <-time.After(time.Second):
					if isResolved(req.UUID) {
						break waiting
					}
					slog.Warn("waiting for approval or disapproval", "id", req.UUID.String())
				}
			}
			if !getResolved(req.UUID).Approved {
				return fmt.Errorf("that tool call was denied") // TODO: more info, AND hook error that allows continue orchard loop
			}
		}
		return nil
	}
	e.NonTextHook = pending.NonText
	e.FinishHook = pending.Finish
	if global.role != "" {
		role, err := roles.ReadRole(global.role)
		if err != nil {
			slog.Error("error streaming", "err", err)
			go global.SendFunc(err)
		}
		e.PromptSuffix = role
	}

	// orchard ai - multiple rounds, tool calls
	out, err := orchard.Create(ctx, e)
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
