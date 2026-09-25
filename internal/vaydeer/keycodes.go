package vaydeer

import "fmt"

// KeyCode is one key the keypad can send. The keypad stores Windows virtual-key codes.
type KeyCode struct {
	Code     byte   `json:"code"`
	Name     string `json:"name"`
	Group    string `json:"group"`
	Modifier bool   `json:"modifier,omitempty"`
}

// KeyCodes lists the keys offered in the editor. F13-F24 are left out: on this keypad's
// firmware they are reported not to send anything.
var KeyCodes = buildKeyCodes()

func buildKeyCodes() []KeyCode {
	var l []KeyCode
	add := func(group string, code byte, name string) {
		l = append(l, KeyCode{Code: code, Name: name, Group: group})
	}

	for _, m := range []struct {
		code byte
		name string
	}{{17, "Ctrl"}, {16, "Shift"}, {18, "Alt"}, {91, "Win"}} {
		l = append(l, KeyCode{Code: m.code, Name: m.name, Group: "Modifier", Modifier: true})
	}
	for c := byte('0'); c <= '9'; c++ {
		add("Digit", c, string(rune(c)))
	}
	for c := byte('A'); c <= 'Z'; c++ {
		add("Letter", c, string(rune(c)))
	}
	for i := byte(1); i <= 12; i++ {
		add("Function", 111+i, fmt.Sprintf("F%d", i))
	}
	for _, k := range []struct {
		code byte
		name string
	}{
		{13, "Enter"}, {27, "Esc"}, {32, "Space"}, {9, "Tab"}, {8, "Backspace"}, {46, "Delete"}, {45, "Insert"},
		{36, "Home"}, {35, "End"}, {33, "Page Up"}, {34, "Page Down"},
		{37, "Left"}, {38, "Up"}, {39, "Right"}, {40, "Down"},
		{44, "Print Screen"}, {19, "Pause"}, {20, "Caps Lock"}, {144, "Num Lock"}, {145, "Scroll Lock"}, {93, "Menu"},
	} {
		add("Navigation and editing", k.code, k.name)
	}
	for _, k := range []struct {
		code byte
		name string
	}{
		{186, ";"}, {187, "="}, {188, ","}, {189, "-"}, {190, "."}, {191, "/"}, {192, "`"}, {219, "["}, {220, `\`}, {221, "]"}, {222, "'"},
	} {
		add("Punctuation (US layout)", k.code, k.name)
	}
	for i := byte(0); i <= 9; i++ {
		add("Number pad", 96+i, fmt.Sprintf("Num %d", i))
	}
	for _, k := range []struct {
		code byte
		name string
	}{{106, "Num *"}, {107, "Num +"}, {109, "Num -"}, {110, "Num ."}, {111, "Num /"}} {
		add("Number pad", k.code, k.name)
	}
	for _, k := range []struct {
		code byte
		name string
	}{
		{179, "Play/Pause"}, {178, "Stop"}, {176, "Next track"}, {177, "Previous track"},
		{175, "Volume up"}, {174, "Volume down"}, {173, "Mute"},
	} {
		add("Media", k.code, k.name)
	}
	return l
}

// KeyName is the display name of a code, or its number when unknown.
func KeyName(code byte) string {
	for _, k := range KeyCodes {
		if k.Code == code {
			return k.Name
		}
	}
	return fmt.Sprintf("code %d", code)
}
