package peco

import (
	"sync"
	"time"

	"github.com/nsf/termbox-go"
)

type Input struct {
	*Ctx
	mutex         *sync.Mutex // Currently only used for protecting Alt/Esc workaround
	mod           *time.Timer
	currentKeymap Keymap
	chained       bool
}

func (i *Input) Loop() {
	defer i.ReleaseWaitGroup()

	// XXX termbox.PollEvent() can get stuck on unexpected signal
	// handling cases. We still would like to wait until the user
	// (termbox) has some event for us to process, but we don't
	// want to allow termbox to control/block our input loop.
	//
	// Solution: put termbox polling in a separate goroutine,
	// and we just watch for a channel. The loop can now
	// safely be implemented in terms of select {} which is
	// safe from being stuck.
	evCh := make(chan termbox.Event)
	go func() {
		for {
			evCh <- termbox.PollEvent()
		}
	}()

	for {
		select {
		case <-i.LoopCh(): // can only fall here if we closed c.loopCh
			return
		case ev := <-evCh:
			i.handleInputEvent(ev)
		}
	}
}

func (i *Input) handleInputEvent(ev termbox.Event) {
	hasModifierMaps := ActiveKeymap.hasModifierMaps()
	switch ev.Type {
	case termbox.EventError:
		//update = false
	case termbox.EventResize:
		i.DrawMatches(nil)
	case termbox.EventKey:
		kev := KeyEvent{ev, i}
		// ModAlt is a sequence of letters with a leading \x1b (=Esc).
		// It would be nice if termbox differentiated this for us, but
		// we workaround it by waiting (juuuuse a few milliseconds) for
		// extra key events. If no extra events arrive, it should be Esc
		if !hasModifierMaps {
			ActiveKeymap.Current().HandleEvent(kev)
			return
		}

		// Smells like Esc or Alt. mod == nil checks for the presense
		// of a previous timer
		if ev.Ch == 0 && ev.Key == 27 && i.mod == nil {
			tmp := kev
			i.mutex.Lock()
			i.mod = time.AfterFunc(50*time.Millisecond, func() {
				i.mutex.Lock()
				i.mod = nil
				i.mutex.Unlock()
				ActiveKeymap.Current().HandleEvent(tmp)
			})
			i.mutex.Unlock()
		} else {
			// it doesn't look like this is Esc or Alt. If we have a previous
			// timer, stop it because this is probably Alt+ this new key
			i.mutex.Lock()
			if i.mod != nil {
				i.mod.Stop()
				i.mod = nil
				ev.Mod |= ModAlt
			}
			i.mutex.Unlock()
			ActiveKeymap.Current().HandleEvent(kev)
		}
	}
}
