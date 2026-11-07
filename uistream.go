package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"github.com/aerth/mostly/mllm"
)

// PendingStreamTUI collects the streaming llm response, periodically (quickly) sending to UI thread
//
// Caller must assign NonText, PreText, Finish to [orchard.Config] to work properly.
//
// See also: hgr hunter gatherer, which uses a different technique, (ui thread pull from channel)
type PendingStreamTUI struct {
	pre  mllm.ChatMessages
	post mllm.ChatMessages
	cr   mllm.ChatResponse // to collect text/errors/stats
	cur  *mllm.ChatMessage // ptr, with MessageID, to post item set by [Pending.PreText] copied from cr each frame

	// keep after reset

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

// bghandler is a [mllm.StreamHandler] collecting text to the current p.cur, p.cr
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
	if cr.Done || time.Since(global.lastsent) > global.Refresh {
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
