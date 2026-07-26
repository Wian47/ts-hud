// internal/ui/sshkeys_test.go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyMsgToBytes(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want []byte
	}{
		{"lowercase rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, []byte("a")},
		{"unicode rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")}, []byte("é")},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, []byte(" ")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, []byte("\r")},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, []byte("\t")},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, []byte{0x7f}},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, []byte{0x1b}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, []byte("\x1b[A")},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, []byte("\x1b[B")},
		{"left", tea.KeyMsg{Type: tea.KeyLeft}, []byte("\x1b[D")},
		{"right", tea.KeyMsg{Type: tea.KeyRight}, []byte("\x1b[C")},
		{"home", tea.KeyMsg{Type: tea.KeyHome}, []byte("\x1b[H")},
		{"end", tea.KeyMsg{Type: tea.KeyEnd}, []byte("\x1b[F")},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, []byte("\x1b[5~")},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, []byte("\x1b[6~")},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, []byte("\x1b[3~")},
		{"ctrl+a", tea.KeyMsg{Type: tea.KeyCtrlA}, []byte{0x01}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, []byte{0x03}},
		{"ctrl+z", tea.KeyMsg{Type: tea.KeyCtrlZ}, []byte{0x1a}},
		{"alt+rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}, []byte{0x1b, 'b'}},
		{"unsupported", tea.KeyMsg{Type: tea.KeyF1}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keyMsgToBytes(tt.msg)
			if string(got) != string(tt.want) {
				t.Errorf("keyMsgToBytes(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}
