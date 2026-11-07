package main

// A simple program demonstrating the text area component from the Bubbles
// component library.

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var cancelGen = func() {}

var welcomemessage = "\n\n\nWelcome to Orchard AI (Beta)\n" +
	`
Copyright (c) 2026 aerth

All Rights Reserved.

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
` + "\n--------\nWARNING!!\nThis program is dangerous!" +
	"\n\nThere are no safety protocols in place." +
	"\nThe \"AI Agent\" is able to run shell commands without human confirmation." +
	"\nYou have been warned!" +
	"\nSource: https://github.com/aerth/orchard" +
	"\n\nPress ENTER to proceed. Ctrl+Q to quit." +
	""

// var actualconvo mllm.ChatMessages
var (
// gdb        *gorm.DB
)

var global = struct {
	gdb      *gorm.DB
	lastsent time.Time
	SendFunc func(tea.Msg)
	// cancelchan chan os.Signal
}{}

func opendb(dbfile string) {
	if dbfile == "" {
		return
	}
	db, err := gorm.Open(sqlite.Open(dbfile), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}
	global.gdb = db
	if err := db.AutoMigrate(&mllm.ChatMessage{}); err != nil {
		closedb()
		log.Fatalln(err)
	}
}

func main() {
	mllm.DefaultThread = uuid.MustParse("aaaa319e-6d4b-4e07-9703-b9baac13ffaa")
	mllm.Flags()
	var dbfile string
	flag.StringVar(&dbfile, "db", dbfile, "path to sqlite db file (optional, for persistence)")
	flag.Parse()
	if dbfile != "" {
		opendb(dbfile)
	}

	if err := main2(); err != nil {
		slog.Error("fatal", "err", err)
		return
	}
}

func main2() error {
	if global.gdb != nil {
		defer closedb()
	}
	if !mllm.Quiet {
		// log stuff
		logfile, err := os.OpenFile("/tmp/orchard.json.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
		if err != nil {
			log.Printf("fatal: %v", err)
		}
		defer func() {
			_ = logfile.Close()
			println("closed log file")
		}()
		slog.SetDefault(slog.New(slog.NewJSONHandler(logfile, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})))
	}
	var dbx gorm.CreateInterface[mllm.ChatMessage]
	if global.gdb != nil {
		dbx = gorm.G[mllm.ChatMessage](global.gdb)
	}
	ocfg := orchard.Config{
		E:            mllm.Default(),
		Think:        false,
		ThreadReader: nil,
		DB:           dbx,
		Toolbox:      orchard.AllTools,
	}
	p := tea.NewProgram(initialModel(ocfg), tea.WithoutSignalHandler())
	global.SendFunc = p.Send // does block
	cancelchan := make(chan os.Signal, 10)
	signal.Notify(cancelchan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		x := 0
		var t1 time.Time
		for sig := range cancelchan {
			x++
			if time.Since(t1) > time.Second*6 {
				x = 1
				t1 = time.Now()
			}
			slog.Error("got signal", "also", len(cancelchan), "x", x)
			if x > 3 && time.Since(t1) < time.Second*6 {
				slog.Error("too many signals")
				global.SendFunc(tea.QuitMsg{})
			}
			t1 = time.Now()
			go global.SendFunc(sig)
		}
	}()
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

func closedb() {
	if global.gdb == nil {
		return
	}
	sqldb, _ := global.gdb.DB()
	if sqldb != nil {
		_ = sqldb.Close()
		println("closed db")
	}
}

type model struct {
	viewport                viewport.Model
	messages                []string
	convo                   mllm.ChatMessages
	textarea                textarea.Model
	senderStyle, errorStyle lipgloss.Style
	err                     error
	ocfg                    orchard.Config
	threadID                uuid.UUID
	userName                string // bob
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

	ta.ShowLineNumbers = true

	vp := viewport.New(viewport.WithWidth(30), viewport.WithHeight(5))
	vp.KeyMap.Left.SetEnabled(false)
	vp.KeyMap.Right.SetEnabled(false)
	vp.SoftWrap = true

	ta.KeyMap.InsertNewline.SetEnabled(false)

	if mllm.DefaultThread == uuid.Nil() {
		slog.Error("nil thread ID")
	} else {
		slog.Info("welcome thread", "id", mllm.DefaultThread)
	}
	var convo mllm.ChatMessages
	if ocfg.DB != nil {
		got, err := ocfg.GormThreadReader(context.TODO(), ocfg.ThreadID)
		if err != nil {
			slog.Warn("thread reader", "err", err)
		}
		if len(got) != 0 {
			convo = got
		}
	}
	vp.SetContent(welcomemessage)
	x := model{
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		errorStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		err:         nil,
		userName:    "Bob",
		threadID:    mllm.DefaultThread,
		ocfg:        ocfg,
		convo:       convo,
	}
	if false && len(convo) != 0 {
		x.messages = renderConvo(convo)
	}
	return x
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

// pendingMessage  mllm.ChatMessages
// pendingResponse mllm.ChatResponse // actual
// pendingAtIndex  = -1
// pendingMessages mllm.ChatMessages

func (m model) WithError(v error) model {
	latest := m.errorStyle.Render("Error:") + " " + v.Error()
	return m.WithMessage(latest)
}

func (m model) WithMessage(v string) model {
	latest := mllm.Message{}.ToThreadMessage("", m.threadID)
	latest.Aux = v
	m.convo = append(m.convo, *latest)
	m.messages = renderConvo(m.convo) // TODO: optimize again
	m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
	m.viewport.GotoBottom()
	return m
}

func renderConvo(convo mllm.ChatMessages) []string {
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

type (
	NewAppendMessage mllm.ChatMessage
	Frag             struct {
		ID   uuid.UUID
		M    mllm.ChatMessage
		Done bool
	}
)

func FormatMessage(cm mllm.ChatMessage) string {
	if cm.Aux != nil {
		return fmt.Sprint(cm.Aux)
	}
	if cm.IsEmpty() {
		return ""
	}
	if cm.Role != "" && cm.Role != "assistant" {
		return strings.Title(cm.Role) + ": " + cm.Content
	}
	if cm.Role == "" {
		cm.Role = "???"
	}
	if cm.Content == "" && len(cm.ToolCalls) == 0 {
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	default:
		typ := fmt.Sprintf("%T", msg)
		if !strings.HasPrefix(typ, "tea.") && !strings.HasPrefix(typ, "cursor.") {
			slog.Error(fmt.Sprintf("unknown packet %s %#v", typ, msg))
		}
	case struct{}:
		m.messages = renderConvo(m.convo) // TODO: optimize again
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
		slog.Info("is finished")
	case os.Signal:
		cancelGen()
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
	case NewAppendMessage:
		m.convo = append(m.convo, mllm.ChatMessage(msg))
		m.messages = renderConvo(m.convo) // TODO: optimize again
		slog.Info("got message "+fmt.Sprintf("%T %s", msg, msg), "current_convo_size", len(m.convo), "current", len(m.messages), "m", msg, "is_empty", msg.IsBlank())
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
		return m, nil
	case Frag:
		if msg.M.IsBlank() {
			slog.Error("got empty frag", "m", msg.M)
			return m, nil
		} else if !msg.Done {
			slog.Info("got frag ", "atID", msg.ID, "finished", msg.Done, "size", len(m.convo), "content", msg.M.Content, "toolcalls", len(msg.M.ToolCalls))
		}
		//l := len(m.convo)
		at := m.convo.IndexOf(msg.ID)
		if at < 0 {
			slog.Error("frag convo", "convo", len(m.convo), "at", at, "want", msg.ID, "top", m.convo.Top().MessageID)
			return m, nil
		}
		m.convo[at] = msg.M //m.convo[at].Add(msg.M)
		m.convo[at].CreatedAt = time.Now()
		m.messages = renderConvo(m.convo) // TODO: optimize again
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()
		/*	var latest strings.Builder
			out := mllm.NewHandlerOut(&latest, mllm.DefaultThinkPre, mllm.DefaultThinkPost)

			// get the output
			err := out.HandleChat(mllm.ChatResponse{Message: msg.M})
			if err != nil {
				return m.WithError(err), nil
			}*/
		//m.messages[msg.At] = FormatMessage(m.convo[msg.At]) //latest.String()
		//	latest.WriteString(m.senderStyle.Render("AI: " + msg.M.Content))
		/*
			if len(msg.M.ToolCalls) != 0 {
				pendingMessage := msg.M
				latest.WriteString("\n")
				for i := range pendingMessage.ToolCalls {
					latest.WriteString(string(pendingMessage.ToolCalls[i]))
					latest.WriteString("\n")
				}
			}*/
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.textarea.SetWidth(msg.Width)
		m.viewport.SetHeight(msg.Height - m.textarea.Height())

		if len(m.messages) > 0 {
			// Wrap content before setting it.
			m.messages = renderConvo(m.convo) // TODO: optimize again
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
		}
		m.viewport.GotoBottom()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			if len(m.convo) > len(m.messages) {
				return m, tea.Quit
			}
			cancelGen()
			return m, nil
		case "ctrl+q", "esc":
			cancelGen()
			fmt.Println(m.textarea.Value())
			return m, tea.Quit
		case "shift+enter":
			m.textarea.InsertRune('\n')
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			return m, cmd
		case "enter":
			if len(m.convo) > len(m.messages) {
				m.messages = renderConvo(m.convo) // TODO: optimize again
				m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
				return m, nil

			}
			cancelGen()
			val := m.textarea.Value()
			if strings.TrimSpace(val) == "" {
				return m, nil
			}
			usermsg := mllm.Message{Role: "user", Content: val}.ToThreadMessage(m.userName, m.threadID)

			ctx := context.TODO()
			m.convo = append(m.convo, *usermsg)
			m.messages = renderConvo(m.convo) // TODO: optimize again
			m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width()).Render(strings.Join(m.messages, "\n")))
			m.textarea.Reset()
			m.viewport.GotoBottom()
			go startGen(ctx, m.ocfg, slices.Clone(m.convo), &PendingStreamTUI{
				From:   m.ocfg.From,
				Thread: m.threadID,
				pre:    m.convo,
			})
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

// PendingStreamTUI collects the stream and periodically (quickly) sends to UI thread
//
// Caller must assign NonText, PreText, Finish to [orchard.Config] to work properly.
type PendingStreamTUI struct {
	pre  mllm.ChatMessages
	post mllm.ChatMessages
	// at   int // current pending
	cr     mllm.ChatResponse // to collect text/errors/stats
	cur    *mllm.ChatMessage // ptr, with MessageID, to post item set by [Pending.PreText] copied from cr each frame
	From   string
	Thread uuid.UUID
	mu     sync.Mutex
}

// NonText for tool runner outputs (Hook for [orchard.Config])
func (p *PendingStreamTUI) NonText(x mllm.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	newmsg := x.ToThreadMessage(p.From, p.Thread)
	slog.Info("NonText", "m", newmsg)
	p.post = append(p.post, *newmsg)
	go global.SendFunc(NewAppendMessage(*newmsg)) // block
	return nil
}

// Finish for when this run is finished (Hook for [orchard.Config])
func (p *PendingStreamTUI) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	//p.reset()
	//go SendFunc(struct{}{})
}

// PreText pre-generation, append empty (Hook for [orchard.Config])
func (p *PendingStreamTUI) PreText() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	newone := mllm.Message{Role: "assistant", Content: ""}.ToThreadMessage(p.From, p.Thread) // new ID
	l := len(p.post)
	p.post = append(p.post, *newone)
	p.cur = &p.post[l]
	slog.Info("PreText", "sizePre", len(p.pre), "post", len(p.post), "id", newone.MessageID.String())
	go global.SendFunc(NewAppendMessage(*newone)) // block
	return nil
}

func (p *PendingStreamTUI) reset() {
	//slog.Info("resetting", "p", p)
	p.cr = mllm.ChatResponse{}
	p.cur = nil
	p.post = nil
	p.pre = nil
}

// bghandler accepts gen
func (p *PendingStreamTUI) bghandle(cr mllm.ChatResponse) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cur == nil {
		return fmt.Errorf("PreText failed")
	}
	if p.cr.Done { // was previous turn
		slog.Error("previous was not blanked", "m", p.cr.Message)
		p.cr = mllm.ChatResponse{}
	}
	if err := p.cr.Handler(cr); err != nil {
		go global.SendFunc(fmt.Errorf("cr.Handler %w %#v", err, cr))
		return err
	}
	p.cur.Message = p.cr.Message // since p.cur is a different..
	if cr.Done || time.Since(global.lastsent) > time.Second/10 {
		global.lastsent = time.Now()
		if p.cur.IsBlank() {
			slog.Warn("sending blank", "x", p.cr)
		}
		// TODO: other
		global.SendFunc(Frag{M: *p.cur, Done: p.cr.Done, ID: p.cur.MessageID})
	}
	if cr.Done {
		p.cr = mllm.ChatResponse{}
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

func startGen(ctx context.Context, e orchard.Config, actualconvo mllm.ChatMessages, pending *PendingStreamTUI) {
	cancelGen()
	if len(actualconvo) == 0 || actualconvo[0].Role == "" {
		slog.Error("cant stream empty convo", "size", len(actualconvo))
		global.SendFunc(fmt.Errorf("cant stream empty convo"))
		return
	}
	ctx, cancelGen = context.WithCancel(ctx)
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
		slog.Info("created", "x", actualconvo[len(actualconvo)-1], "for_thread", e.ThreadID)
		e.ThreadReader = func(ctx context.Context, threadID uuid.UUID) (mllm.ChatMessages, error) {
			got, err := e.GormThreadReader(ctx, threadID)
			slog.Info("got", "size", len(got), "err", err, "thread", threadID)
			if err == nil && len(got) == 0 {
				err = gorm.ErrRecordNotFound
			}
			if err != nil {
				return nil, err
			}
			return got, nil
		}
	}
	e.Options.Debug = 0
	e.Handlers = append(e.Handlers, pending.bghandle)
	if len(e.Toolbox) == 0 {
		e.Toolbox = orchard.AllTools
	}
	e.PreTextHook = pending.PreText
	e.NonTextHook = pending.NonText
	e.FinishHook = pending.Finish
	out, err := orchard.Create(ctx, e.ThreadID, e)
	//err := <-e.StreamBG(ctx, convo.ToMessages(), e.Think, handler)
	if err != nil {
		slog.Error("error streaming", "err", err)
		go global.SendFunc(err)
		return
	}
	_ = out
	pending.reset()
	slog.Info("got out", "n", len(out))
}

func (m model) View() tea.View {
	viewportView := m.viewport.View()
	v := tea.NewView(viewportView + "\n" + m.textarea.View())
	c := m.textarea.Cursor()
	if c != nil {
		c.Y += lipgloss.Height(viewportView)
	}
	v.Cursor = c
	v.AltScreen = false
	return v
}

var _ tea.Model = model{}
