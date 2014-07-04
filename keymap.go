package peco

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nsf/termbox-go"
)

// Possible key modifiers
const (
	ModNone = iota
	ModAlt
	ModMax
)

// ActiveKeymap is the currently active keymap struct
var ActiveKeymap Keymap

type KeyEvent struct {
	termbox.Event
	input *Input
}

type KeySeq struct {
	key termbox.Key
	mod int
}

type Keymap struct {
	root    KeymapNodeWithMod
	current *KeymapNodeWithMod
}

// action, err := ActiveKeymap.Current().Get(ev)
// action.Execute(i, ev)
func (m Keymap) Current() KeymapNodeWithMod {
	if m.current == nil {
		return m.root
	}
	return *m.current
}

func (m Keymap) IsChained() bool {
	return m.current != nil
}

type KeymapNodeWithMod [ModMax]KeymapNode
type KeymapNode map[termbox.Key]interface{}

func (m KeymapNodeWithMod) GetItem(ev termbox.Event) (interface{}, error) {
	modifier := ModNone
	if (ev.Mod & termbox.ModAlt) != 0 {
		modifier = ModAlt
	}

	// RawKeymap that we will be using
	rkm := m[modifier]

	switch modifier {
	case ModAlt, ModNone:
		var key termbox.Key
		if ev.Ch == 0 {
			key = ev.Key
		} else {
			key = termbox.Key(ev.Ch)
		}

		if i, ok := rkm[key]; ok {
			return i, nil
		}
	default:
		// Can't get here
		return nil, fmt.Errorf("Invalid modifier")
	}

	return nil, fmt.Errorf("No matching item")
}

func (m KeymapNodeWithMod) GetAction(ev termbox.Event) Action {
	item, err := m.GetItem(ev)
	if err != nil {
		if ActiveKeymap.IsChained() {
			return ActionFunc(handleResetKeySequence)
		}
		return ActionFunc(handleAcceptChar)
	}

	switch item.(type) {
	case KeymapNodeWithMod:
		// XXX should fire a timer
		return nil
	case Action:
		return item.(Action)
	}
	return nil
}

func (m KeymapNodeWithMod) HandleEvent(ev KeyEvent) {
	action := m.GetAction(ev.Event)
	if action == nil {
		return
	}

	action.Execute(ev.input, ev.Event)
}

type KeymapStringKey string

// This map is populated using some magic numbers, which must match
// the values defined in termbox-go. Verification against the actual
// termbox constants are done in the test
var stringToKey = map[string]termbox.Key{}

func init() {
	fidx := 12
	for k := termbox.KeyF1; k > termbox.KeyF12; k-- {
		sk := fmt.Sprintf("F%d", fidx)
		stringToKey[sk] = k
		fidx--
	}

	names := []string{
		"Insert",
		"Delete",
		"Home",
		"End",
		"Pgup",
		"Pgdn",
		"ArrowUp",
		"ArrowDown",
		"ArrowLeft",
		"ArrowRight",
	}
	for i, n := range names {
		stringToKey[n] = termbox.Key(int(termbox.KeyF12) - (i + 1))
	}

	names = []string{
		"Left",
		"Middle",
		"Right",
	}
	for i, n := range names {
		sk := fmt.Sprintf("Mouse%s", n)
		stringToKey[sk] = termbox.Key(int(termbox.KeyArrowRight) - (i + 2))
	}

	whacky := [][]string{
		{"~", "2", "Space"},
		{"a"},
		{"b"},
		{"c"},
		{"d"},
		{"e"},
		{"f"},
		{"g"},
		{"h"},
		{"i"},
		{"j"},
		{"k"},
		{"l"},
		{"m"},
		{"n"},
		{"o"},
		{"p"},
		{"q"},
		{"r"},
		{"s"},
		{"t"},
		{"u"},
		{"v"},
		{"w"},
		{"x"},
		{"y"},
		{"z"},
		{"[", "3"},
		{"4", "\\"},
		{"5", "]"},
		{"6"},
		{"7", "/", "_"},
	}
	for i, list := range whacky {
		for _, n := range list {
			sk := fmt.Sprintf("C-%s", n)
			stringToKey[sk] = termbox.Key(int(termbox.KeyCtrlTilde) + i)
		}
	}

	stringToKey["BS"] = termbox.KeyBackspace
	stringToKey["Tab"] = termbox.KeyTab
	stringToKey["Enter"] = termbox.KeyEnter
	stringToKey["Esc"] = termbox.KeyEsc
	stringToKey["Space"] = termbox.KeySpace
	stringToKey["BS2"] = termbox.KeyBackspace2
	stringToKey["C-8"] = termbox.KeyCtrl8

	//	panic(fmt.Sprintf("%#q", stringToKey))
}

func handleAcceptChar(i *Input, ev termbox.Event) {
	if ev.Key == termbox.KeySpace {
		ev.Ch = ' '
	}

	if ev.Ch > 0 {
		if len(i.query) == i.caretPos {
			i.query = append(i.query, ev.Ch)
		} else {
			buf := make([]rune, len(i.query)+1)
			copy(buf, i.query[:i.caretPos])
			buf[i.caretPos] = ev.Ch
			copy(buf[i.caretPos+1:], i.query[i.caretPos:])
			i.query = buf
		}
		i.caretPos++
		i.ExecQuery()
	}
}

func handleResetKeySequence(i *Input, ev termbox.Event) {
//	i.currentKeymap = i.config.Keymap
//	i.chained = false
}

func (ksk KeymapStringKey) ToKey() (k termbox.Key, modifier int, err error) {
	modifier = ModNone
	key := string(ksk)
	if strings.HasPrefix(key, "M-") {
		modifier = ModAlt
		key = key[2:]
		if len(key) == 1 {
			k = termbox.Key(key[0])
			return
		}
	}

	var ok bool
	k, ok = stringToKey[key]
	if !ok {
		err = fmt.Errorf("No such key %s", ksk)
	}
	return
}

func NewKeymap() Keymap {
	def := KeymapNode{}
	for k, v := range defaultKeyBinding {
		def[k] = v
	}
	return Keymap{
		KeymapNodeWithMod{
			def,
			KeymapNode{},
		},
		nil,
	}
}

func (km *Keymap) UnmarshalJSON(buf []byte) error {
	raw := map[string]interface{}{}
	if err := json.Unmarshal(buf, &raw); err != nil {
		return err
	}

	km.assignKeyHandlers(raw)
	return nil
}

func (km *Keymap) assignKeyHandlers(raw map[string]interface{}) {
	for ks, vi := range raw {
fmt.Fprintf(os.Stderr, "ks = %v, vi = %v\n", ks, vi)
		k, modifier, err := KeymapStringKey(ks).ToKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unknown key %s", ks)
			continue
		}

		keymap := km.root[modifier]
		if keymap == nil {
			keymap = KeymapNode{}
			km.root[modifier] = keymap
		}

		switch vi.(type) {
		case string:
			vs := vi.(string)
			if vs == "-" {
				delete(keymap, k)
				continue
			}

fmt.Fprintf(os.Stderr, "vi is a string\n")
			v, ok := nameToActions[vs]
			if !ok {
				fmt.Fprintf(os.Stderr, "Unknown handler %s", vs)
				continue
			}
			keymap[k] = ActionFunc(func(i *Input, ev termbox.Event) {
				v.Execute(i, ev)

				// Reset key sequence when not-chained key was pressed
				handleResetKeySequence(i, ev)
			})
		case map[string]interface{}:
			ckm := Keymap{KeymapNodeWithMod{}, nil}
			ckm.assignKeyHandlers(vi.(map[string]interface{}))
			keymap[k] = ActionFunc(func(i *Input, _ termbox.Event) {
				// Switch Keymap for chained state
				i.currentKeymap = ckm
				i.chained = true
			})
		}
	}
}

func (km Keymap) hasModifierMaps() bool {
	current := km.Current()
	return len(current[ModAlt]) > 0
}

// func (km Keymap) SetActionForSequence(a Action, keys ...KeySeq) error {
