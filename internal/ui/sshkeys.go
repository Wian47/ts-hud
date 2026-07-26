package ui

import tea "github.com/charmbracelet/bubbletea"

// keyMsgToBytes encodes a subset of tea.KeyMsg values into the raw bytes an
// ssh session's pty expects on stdin. It is best-effort — the common set
// used by shells, vim, htop, and similar full-screen programs — not a
// byte-perfect terminal input encoder. Mouse events and bracketed paste are
// out of scope.
func keyMsgToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return withAltPrefix(msg.Alt, []byte(string(msg.Runes)))
	case tea.KeySpace:
		return withAltPrefix(msg.Alt, []byte(" "))
	case tea.KeyEnter:
		return withAltPrefix(msg.Alt, []byte("\r"))
	case tea.KeyTab:
		return withAltPrefix(msg.Alt, []byte("\t"))
	case tea.KeyBackspace:
		return withAltPrefix(msg.Alt, []byte{0x7f})
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	}
	if msg.Type >= tea.KeyCtrlA && msg.Type <= tea.KeyCtrlZ {
		return []byte{byte(msg.Type)}
	}
	return nil
}

// withAltPrefix prepends the ESC byte that terminals use to signal an
// alt-modified key, when alt was held.
func withAltPrefix(alt bool, b []byte) []byte {
	if !alt {
		return b
	}
	return append([]byte{0x1b}, b...)
}
