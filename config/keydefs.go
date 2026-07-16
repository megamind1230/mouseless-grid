package config

import "slices"

// evdev keycodes — copied from jbensmann/mouseless/config/keydefs.go
// baka: this maps Linux evdev key code numbers to human-readable names

var keyAliases = map[string]uint16{
	"esc":         1,
	"k1":          2, "k2": 3, "k3": 4, "k4": 5, "k5": 6,
	"k6":          7, "k7": 8, "k8": 9, "k9": 10, "k0": 11,
	"minus":       12, "equal": 13, "backspace": 14, "tab": 15,
	"q": 16, "w": 17, "e": 18, "r": 19, "t": 20,
	"y": 21, "u": 22, "i": 23, "o": 24, "p": 25,
	"leftbrace": 26, "rightbrace": 27, "enter": 28, "leftctrl": 29,
	"a": 30, "s": 31, "d": 32, "f": 33, "g": 34,
	"h": 35, "j": 36, "k": 37, "l": 38,
	"semicolon": 39, "apostrophe": 40, "grave": 41,
	"leftshift": 42, "backslash": 43,
	"z": 44, "x": 45, "c": 46, "v": 47, "b": 48,
	"n": 49, "m": 50, "comma": 51, "dot": 52, "slash": 53,
	"rightshift": 54, "kpasterisk": 55, "leftalt": 56, "space": 57,
	"capslock": 58,
	"f1": 59, "f2": 60, "f3": 61, "f4": 62, "f5": 63, "f6": 64,
	"f7": 65, "f8": 66, "f9": 67, "f10": 68,
	"numlock": 69, "scrolllock": 70,
	"kp7": 71, "kp8": 72, "kp9": 73, "kpminus": 74,
	"kp4": 75, "kp5": 76, "kp6": 77, "kpplus": 78,
	"kp1": 79, "kp2": 80, "kp3": 81, "kp0": 82, "kpdot": 83,
	"f11": 87, "f12": 88,
	"kpenter": 96, "rightctrl": 97, "kpslash": 98, "rightalt": 100,
	"home": 102, "up": 103, "pageup": 104,
	"left": 105, "right": 106, "end": 107, "down": 108, "pagedown": 109,
	"insert": 110, "delete": 111,
	"mute": 113, "volumedown": 114, "volumeup": 115,
	"leftmeta": 125, "rightmeta": 126,
}

var keyAliasesReversed = make(map[uint16]string)

var modifierKeys = []string{
	"leftshift", "rightshift", "leftctrl", "rightctrl",
	"leftalt", "rightalt", "leftmeta", "rightmeta",
}
var modifiersKeyCodes []uint16

func init() {
	for alias, code := range keyAliases {
		keyAliasesReversed[code] = alias
	}
	for _, modifier := range modifierKeys {
		code, exists := keyAliases[modifier]
		if !exists {
			panic("unknown modifier: " + modifier)
		}
		modifiersKeyCodes = append(modifiersKeyCodes, code)
	}
}

func GetKeyCode(alias string) (code uint16, exists bool) {
	code, exists = keyAliases[alias]
	return
}

func GetKeyAlias(code uint16) (alias string, exists bool) {
	alias, exists = keyAliasesReversed[code]
	return
}

func IsModifierKey(code uint16) bool {
	return slices.Contains(modifiersKeyCodes, code)
}
