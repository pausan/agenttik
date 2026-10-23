package screen

import (
	"strings"
	"unicode/utf8"
)

// X keysyms are the vocabulary every Display speaks: they name what a key
// types rather than where it sits, so a viewer on one keyboard layout can
// drive a machine on another.

// KeysymForRune is the keysym that types r: Latin-1 maps to itself and
// everything else to the Unicode range.
func KeysymForRune(r rune) uint32 {
	switch {
	case r == '\n' || r == '\r':
		return 0xff0d // Return
	case r == '\t':
		return 0xff09 // Tab
	case r == '\b':
		return 0xff08 // BackSpace
	case r >= 0x20 && r <= 0x7e, r >= 0xa0 && r <= 0xff:
		return uint32(r)
	}
	return 0x01000000 | uint32(r)
}

// browserKeys maps KeyboardEvent.key names, and the codes that tell left
// from right, to keysyms.
var browserKeys = map[string]uint32{
	"Backspace": 0xff08, "Tab": 0xff09, "Enter": 0xff0d, "Escape": 0xff1b,
	"Delete": 0xffff, "Home": 0xff50, "ArrowLeft": 0xff51, "ArrowUp": 0xff52,
	"ArrowRight": 0xff53, "ArrowDown": 0xff54, "PageUp": 0xff55,
	"PageDown": 0xff56, "End": 0xff57, "Insert": 0xff63, "ContextMenu": 0xff67,
	"Pause": 0xff13, "ScrollLock": 0xff14, "PrintScreen": 0xff61,
	"NumLock": 0xff7f, "CapsLock": 0xffe5, " ": 0x20,
	"ShiftLeft": 0xffe1, "ShiftRight": 0xffe2, "Shift": 0xffe1,
	"ControlLeft": 0xffe3, "ControlRight": 0xffe4, "Control": 0xffe3,
	"AltLeft": 0xffe9, "AltRight": 0xffea, "Alt": 0xffe9,
	"MetaLeft": 0xffeb, "MetaRight": 0xffec, "Meta": 0xffeb, "OSLeft": 0xffeb, "OSRight": 0xffec,
	"AudioVolumeMute": 0x1008ff12, "AudioVolumeDown": 0x1008ff11, "AudioVolumeUp": 0x1008ff13,
}

// KeysymForBrowserKey reads a browser key event: `key` says what it types and
// `code` where it is, which only matters for telling left modifiers from
// right ones and for keys whose `key` is a character the layout produced.
func KeysymForBrowserKey(key, code string) (uint32, bool) {
	if ks, ok := browserKeys[code]; ok && isModifier(ks) {
		return ks, true
	}
	if ks, ok := browserKeys[key]; ok {
		return ks, true
	}
	if len(key) >= 2 && key[0] == 'F' {
		if n := atoi(key[1:]); n >= 1 && n <= 24 {
			return 0xffbe + uint32(n-1), true
		}
	}
	if r, size := utf8.DecodeRuneInString(key); size == len(key) && r != utf8.RuneError {
		return KeysymForRune(r), true
	}
	return 0, false
}

func isModifier(ks uint32) bool { return ks >= 0xffe1 && ks <= 0xffee }

// actionKeys are the names agents use for keys, as in xdotool and the
// computer-use tool: "ctrl+shift+t", "Return", "super".
var actionKeys = map[string]uint32{
	"ctrl": 0xffe3, "control": 0xffe3, "control_l": 0xffe3, "control_r": 0xffe4,
	"shift": 0xffe1, "shift_l": 0xffe1, "shift_r": 0xffe2,
	"alt": 0xffe9, "alt_l": 0xffe9, "alt_r": 0xffea, "option": 0xffe9,
	"super": 0xffeb, "super_l": 0xffeb, "super_r": 0xffec, "win": 0xffeb, "meta": 0xffeb, "cmd": 0xffeb, "command": 0xffeb,
	"return": 0xff0d, "enter": 0xff0d, "kp_enter": 0xff8d, "tab": 0xff09, "escape": 0xff1b, "esc": 0xff1b,
	"backspace": 0xff08, "delete": 0xffff, "del": 0xffff, "insert": 0xff63,
	"space": 0x20, "home": 0xff50, "end": 0xff57,
	"pageup": 0xff55, "page_up": 0xff55, "prior": 0xff55, "pagedown": 0xff56, "page_down": 0xff56, "next": 0xff56,
	"left": 0xff51, "up": 0xff52, "right": 0xff53, "down": 0xff54,
	"menu": 0xff67, "print": 0xff61, "capslock": 0xffe5, "caps_lock": 0xffe5,
	"minus": '-', "plus": '+', "equal": '=', "comma": ',', "period": '.', "slash": '/',
	"backslash": '\\', "semicolon": ';', "apostrophe": '\'', "grave": '`',
	"bracketleft": '[', "bracketright": ']',
}

// ParseKeyCombo reads "ctrl+shift+t" into keysyms, pressed in order and
// released in reverse.
func ParseKeyCombo(combo string) ([]uint32, bool) {
	parts := strings.Split(combo, "+")
	keys := make([]uint32, 0, len(parts))
	for i, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			// "ctrl++" means the plus key itself.
			if i == len(parts)-1 && i > 0 {
				name = "+"
			} else {
				continue
			}
		}
		ks, ok := keysymForName(name)
		if !ok {
			return nil, false
		}
		keys = append(keys, ks)
	}
	return keys, len(keys) > 0
}

func keysymForName(name string) (uint32, bool) {
	if ks, ok := actionKeys[strings.ToLower(name)]; ok {
		return ks, true
	}
	lower := strings.ToLower(name)
	if len(lower) >= 2 && lower[0] == 'f' {
		if n := atoi(lower[1:]); n >= 1 && n <= 24 {
			return 0xffbe + uint32(n-1), true
		}
	}
	if r, size := utf8.DecodeRuneInString(name); size == len(name) && r != utf8.RuneError {
		return KeysymForRune(r), true
	}
	if ks, ok := browserKeys[name]; ok {
		return ks, true
	}
	return 0, false
}

func atoi(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
