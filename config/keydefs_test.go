package config

import (
	"testing"
)

func TestGetKeyCode(t *testing.T) {
	tests := []struct {
		alias    string
		wantCode uint16
		wantOK   bool
	}{
		{"k1", 2, true},
		{"k9", 10, true},
		{"esc", 1, true},
		{"enter", 28, true},
		{"leftctrl", 29, true},
		{"leftalt", 56, true},
		{"space", 57, true},
		{"nonexistent", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		code, ok := GetKeyCode(tt.alias)
		if code != tt.wantCode || ok != tt.wantOK {
			t.Errorf("GetKeyCode(%q) = (%d, %v), want (%d, %v)",
				tt.alias, code, ok, tt.wantCode, tt.wantOK)
		}
	}
}

func TestGetKeyAlias(t *testing.T) {
	tests := []struct {
		code     uint16
		wantAlias string
		wantOK   bool
	}{
		{2, "k1", true},
		{10, "k9", true},
		{1, "esc", true},
		{28, "enter", true},
		{29, "leftctrl", true},
		{999, "", false},
	}
	for _, tt := range tests {
		alias, ok := GetKeyAlias(tt.code)
		if alias != tt.wantAlias || ok != tt.wantOK {
			t.Errorf("GetKeyAlias(%d) = (%q, %v), want (%q, %v)",
				tt.code, alias, ok, tt.wantAlias, tt.wantOK)
		}
	}
}

func TestIsModifierKey(t *testing.T) {
	mods := []uint16{29, 56, 97, 100, 42, 54, 125, 126} // leftctrl, leftalt, rightctrl, rightalt, leftshift, rightshift, leftmeta, rightmeta
	for _, code := range mods {
		if !IsModifierKey(code) {
			t.Errorf("IsModifierKey(%d) should be true", code)
		}
	}
	nonMods := []uint16{1, 2, 28, 57, 16}
	for _, code := range nonMods {
		if IsModifierKey(code) {
			t.Errorf("IsModifierKey(%d) should be false", code)
		}
	}
}

func TestGetKeyCodeRoundTrip(t *testing.T) {
	// verify that every alias in the map round-trips correctly
	for alias, code := range keyAliases {
		gotCode, ok := GetKeyCode(alias)
		if !ok || gotCode != code {
			t.Errorf("GetKeyCode(%q) = (%d, %v), want (%d, true)", alias, gotCode, ok, code)
		}
		gotAlias, ok := GetKeyAlias(code)
		if !ok || gotAlias != alias {
			t.Errorf("GetKeyAlias(%d) = (%q, %v), want (%q, true)", code, gotAlias, ok, alias)
		}
	}
}

func TestModifierKeysAllExist(t *testing.T) {
	for _, mod := range modifierKeys {
		_, ok := keyAliases[mod]
		if !ok {
			t.Errorf("modifier key %q not found in keyAliases", mod)
		}
	}
}
