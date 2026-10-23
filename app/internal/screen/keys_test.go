package screen

import (
	"slices"
	"testing"
)

func TestKeysymForBrowserKey(t *testing.T) {
	for _, c := range []struct {
		key, code string
		want      uint32
	}{
		{"a", "KeyA", 'a'},
		{"A", "KeyA", 'A'},
		{"é", "Quote", 0xe9},
		{"€", "KeyE", 0x010020ac},
		{"Enter", "Enter", 0xff0d},
		{"Shift", "ShiftRight", 0xffe2},
		{"Control", "ControlLeft", 0xffe3},
		{"Meta", "MetaLeft", 0xffeb},
		{"F5", "F5", 0xffc2},
		{" ", "Space", 0x20},
		{"ArrowUp", "ArrowUp", 0xff52},
	} {
		got, ok := KeysymForBrowserKey(c.key, c.code)
		if !ok || got != c.want {
			t.Errorf("%q/%q: %#x %v, want %#x", c.key, c.code, got, ok, c.want)
		}
	}
	if _, ok := KeysymForBrowserKey("Dead", "Quote"); ok {
		t.Error("a dead key types nothing by itself")
	}
}

func TestParseKeyCombo(t *testing.T) {
	for combo, want := range map[string][]uint32{
		"ctrl+s":          {0xffe3, 's'},
		"ctrl+shift+T":    {0xffe3, 0xffe1, 'T'},
		"Return":          {0xff0d},
		"super":           {0xffeb},
		"alt+F4":          {0xffe9, 0xffc1},
		"ctrl++":          {0xffe3, '+'},
		"Page_Down":       {0xff56},
		"ctrl+alt+Delete": {0xffe3, 0xffe9, 0xffff},
	} {
		got, ok := ParseKeyCombo(combo)
		if !ok || !slices.Equal(got, want) {
			t.Errorf("%q: %#x %v, want %#x", combo, got, ok, want)
		}
	}
	if _, ok := ParseKeyCombo("ctrl+nosuchkey"); ok {
		t.Error("unknown key name accepted")
	}
}
