# Help Overlay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `?` key that opens a static help overlay listing every keybinding in ts-hud, grouped by section, closable with `esc`/`?`, available from anywhere except the search input and the embedded ssh session.

**Architecture:** One new `Model` field (`viewingHelp bool`), a single interception point at the top of `Update()`'s `tea.KeyMsg` handling that toggles it, a `viewingHelp`-first case in both the `Update` dispatch switch and the `View()` render switch, and a new static render function in a new file `internal/ui/help.go`.

**Tech Stack:** Go, Bubble Tea (`tea.Msg`/`tea.Cmd`), lipgloss styling (existing `headerStyle`/`rowStyle`/`helpStyle` from `internal/ui/styles.go`).

## Global Constraints

- `?` must be a no-op (must NOT toggle `viewingHelp`) while `m.searching` or `m.viewingSSH` is true — both consume every keystroke verbatim (search text / remote shell input).
- Opening help must not mutate any other `viewing*`/`picking*`/`confirming*` field — closing help must resume exactly the screen that was open before, with no special-case restore logic.
- The overlay content is static (no fetch, no loading state, no cursor) — spec explicitly puts scrolling/pagination and context-sensitive filtering out of scope.
- Follow the existing per-panel file/render pattern (see `internal/ui/accounts.go`, `internal/ui/prefs.go`): a bare render function taking a `width int` and returning a `string`, styled with the existing shared styles — no new styles.

---

### Task 1: Help overlay state, routing, and rendering

**Files:**
- Create: `internal/ui/help.go`
- Create: `internal/ui/help_test.go`
- Modify: `internal/ui/model.go` (add field, interception point, dispatch case, view case, footer hint)
- Modify: `internal/ui/model_test.go` (add routing tests)

**Interfaces:**
- Produces: `renderHelpPanel(width int) string` — pure static render, no other task depends on it existing but Task 2 (README) describes the same content this function renders, so keep them in sync.
- Produces: `Model.viewingHelp bool` field and `Model.updateHelpView(msg tea.KeyMsg) (tea.Model, tea.Cmd)` method.

- [ ] **Step 1: Write the failing render test**

Create `internal/ui/help_test.go`:

```go
package ui

import "strings"

func TestRenderHelpPanelListsAllSections(t *testing.T) {
	out := renderHelpPanel(80)

	sections := []string{
		"Peer table",
		"Search",
		"Ssh session",
		"Exit node picker",
		"DERP latency matrix",
		"Peer detail",
		"Preferences panel",
		"Connection-down confirm",
		"Account switch",
	}
	for _, s := range sections {
		if !strings.Contains(out, s) {
			t.Errorf("renderHelpPanel output missing section %q\noutput:\n%s", s, out)
		}
	}
}

func TestRenderHelpPanelListsKnownKeys(t *testing.T) {
	out := renderHelpPanel(80)

	keys := []string{"j/k", "g/G", "ctrl+q", "l ", "y ", "esc/a"}
	for _, k := range keys {
		if !strings.Contains(out, k) {
			t.Errorf("renderHelpPanel output missing key %q\noutput:\n%s", k, out)
		}
	}
}
```

Add `"testing"` to the import block (alongside `"strings"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestRenderHelpPanel -v`
Expected: FAIL — `renderHelpPanel` undefined (build failure).

- [ ] **Step 3: Write `internal/ui/help.go`**

```go
package ui

import "strings"

// helpSection is one grouped block in the ? help overlay, mirroring one
// of the app's existing panels.
type helpSection struct {
	title string
	rows  []string
}

// helpSections is the single source of truth for the ? overlay's content.
// It intentionally duplicates the per-panel footer hints already rendered
// in Model.View — this is the one place the whole keymap is visible at
// once, so keys are spelled out here even though they also appear inline.
func helpSections() []helpSection {
	return []helpSection{
		{"Peer table", []string{
			"j/k        move down/up",
			"g/G        jump to top/bottom",
			"/          search",
			"enter      ssh into selected peer",
			"x          exit node picker",
			"d          DERP latency matrix",
			"i          peer detail",
			"p          preferences panel",
			"c          connection up/down",
			"a          account switch",
			"r          refresh",
			"q          quit",
		}},
		{"Search", []string{
			"esc        clear and exit search",
		}},
		{"Ssh session", []string{
			"ctrl+q     detach",
		}},
		{"Exit node picker", []string{
			"j/k        move down/up",
			"enter      select highlighted peer",
			"l          toggle allow LAN access",
			"esc/x      cancel",
		}},
		{"DERP latency matrix", []string{
			"r          re-run the check",
			"esc/d      back",
		}},
		{"Peer detail", []string{
			"r          re-run the probe",
			"esc/i      back",
		}},
		{"Preferences panel", []string{
			"j/k        move down/up",
			"enter      toggle highlighted preference",
			"esc/p      back",
		}},
		{"Connection-down confirm", []string{
			"y          confirm bringing the connection down",
			"n/esc      cancel",
		}},
		{"Account switch", []string{
			"j/k        move down/up",
			"enter      switch to highlighted account",
			"esc/a      back",
		}},
	}
}

// renderHelpPanel renders the ? overlay: a static, grouped list of every
// keybinding in the app. Unlike the other panels it never loads or
// errors, so there is no loading/error branch here.
func renderHelpPanel(width int) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("Help"))
	b.WriteString("\n")

	for i, section := range helpSections() {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(rowStyle.Render("  " + section.title))
		b.WriteString("\n")
		for _, row := range section.rows {
			b.WriteString(helpStyle.Render("    " + row))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  esc/? close"))

	return strings.TrimRight(b.String(), "\n")
}
```

(`width` stays as a parameter for interface consistency with
`renderAccountsPanel`/`renderPrefsPanel`, even though this function
doesn't use it — Go will flag it as unused only for unused imports/vars,
not unused parameters, so this compiles fine.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestRenderHelpPanel -v`
Expected: PASS

- [ ] **Step 5: Write the failing routing tests**

Append to `internal/ui/model_test.go` (match the file's existing test
style — look at `TestAccountsOpensAndFetches` and
`TestAccountsEscCloses` immediately above this file's other accounts
tests for the file's `newTestModel()`-based construction and `tea.KeyMsg`
pattern used throughout before writing these):

```go
func TestHelpOpensAndCloses(t *testing.T) {
	m := newTestModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(Model)
	if !m.viewingHelp {
		t.Fatal("expected viewingHelp to be true after '?'")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.viewingHelp {
		t.Fatal("expected viewingHelp to be false after esc")
	}
}

func TestHelpTogglesClosedOnSecondQuestionMark(t *testing.T) {
	m := newTestModel()
	m.viewingHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(Model)
	if m.viewingHelp {
		t.Fatal("expected viewingHelp to be false after second '?'")
	}
}

func TestHelpDoesNotOpenWhileSearching(t *testing.T) {
	m := newTestModel()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(Model)
	if !m.searching {
		t.Fatal("expected searching to be true after '/'")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(Model)
	if m.viewingHelp {
		t.Fatal("expected viewingHelp to stay false while searching")
	}
	if m.searchInput.Value() != "?" {
		t.Fatalf("expected '?' to be forwarded to the search input, got %q", m.searchInput.Value())
	}
}

func TestHelpDoesNotOpenWhileViewingSSH(t *testing.T) {
	m := newTestModel()
	m.viewingSSH = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(Model)
	if m.viewingHelp {
		t.Fatal("expected viewingHelp to stay false while viewingSSH")
	}
}

func TestHelpClosePreservesUnderlyingPanelState(t *testing.T) {
	m := newTestModel()
	m.viewingDERP = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated.(Model)
	if !m.viewingHelp || !m.viewingDERP {
		t.Fatal("expected viewingHelp true and viewingDERP still true after opening help from DERP view")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.viewingHelp {
		t.Fatal("expected viewingHelp false after esc")
	}
	if !m.viewingDERP {
		t.Fatal("expected viewingDERP to still be true after closing help — underlying panel state must be preserved")
	}
}
```

`TestHelpDoesNotOpenWhileViewingSSH` deliberately leaves `m.sshPane` nil:
`updateSSHPane` only forwards the keystroke to the pty when `m.sshPane !=
nil` (see `internal/ui/model.go` around line 622), so this stays safe
without constructing a real pty session.

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestHelp -v`
Expected: FAIL — `m.viewingHelp` undefined (build failure), since the
field and routing don't exist yet.

- [ ] **Step 7: Add the field, interception point, and dispatch case**

In `internal/ui/model.go`, add the field to the `Model` struct (near the
other `viewing*` bools, e.g. right after `viewingAccounts` fields around
line 92-97):

```go
	viewingHelp bool
```

Change the `tea.KeyMsg` case in `Update` (around line 396-417) from:

```go
	case tea.KeyMsg:
		switch {
		case m.searching:
			return m.updateSearch(msg)
		case m.pickingExitNode:
			return m.updateExitNodePicker(msg)
		case m.viewingDERP:
			return m.updateDERPView(msg)
		case m.viewingPeerDetail:
			return m.updatePeerDetailView(msg)
		case m.viewingPrefs:
			return m.updatePrefsView(msg)
		case m.confirmingDown:
			return m.updateConnConfirm(msg)
		case m.viewingAccounts:
			return m.updateAccountsView(msg)
		case m.viewingSSH:
			return m.updateSSHPane(msg)
		default:
			return m.updateNormal(msg)
		}
	}
```

to:

```go
	case tea.KeyMsg:
		if msg.String() == "?" && !m.searching && !m.viewingSSH {
			m.viewingHelp = !m.viewingHelp
			return m, nil
		}
		switch {
		case m.viewingHelp:
			return m.updateHelpView(msg)
		case m.searching:
			return m.updateSearch(msg)
		case m.pickingExitNode:
			return m.updateExitNodePicker(msg)
		case m.viewingDERP:
			return m.updateDERPView(msg)
		case m.viewingPeerDetail:
			return m.updatePeerDetailView(msg)
		case m.viewingPrefs:
			return m.updatePrefsView(msg)
		case m.confirmingDown:
			return m.updateConnConfirm(msg)
		case m.viewingAccounts:
			return m.updateAccountsView(msg)
		case m.viewingSSH:
			return m.updateSSHPane(msg)
		default:
			return m.updateNormal(msg)
		}
	}
```

Add `updateHelpView` next to the other `update*View` methods (e.g. right
after `updateDERPView`):

```go
// updateHelpView only needs to handle "esc": a "?" keypress is already
// intercepted and toggled off before dispatch ever reaches here (see the
// interception point added in this step), so a "?" case here would be
// dead code.
func (m Model) updateHelpView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.viewingHelp = false
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 8: Add the View() case and footer hint**

In `View()` (around line 750), add `viewingHelp` as the first case in the
switch, before `m.viewingSSH`:

```go
	switch {
	case m.viewingHelp:
		body = renderHelpPanel(width)
		footer = helpStyle.Render("esc/? close")
	case m.viewingSSH:
```

Add `? help` to the default footer hint (around line 781):

```go
			footer = helpStyle.Render("j/k move  g/G top/bottom  / search  enter ssh  x exit-node  d derp  i info  p prefs  c connection  a accounts  ? help  r refresh  q quit")
```

- [ ] **Step 9: Run all UI tests**

Run: `go test ./internal/ui/... -v`
Expected: PASS, all tests including the new `TestHelp*` and
`TestRenderHelpPanel*` ones, no other test broken.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/help.go internal/ui/help_test.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat: add ? help overlay listing all keybindings"
```

---

### Task 2: Document the help overlay in the README

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: the exact key list from Task 1's `helpSections()` in
  `internal/ui/help.go` — this task's table rows must match those section
  titles and keys so the two stay in sync (per the spec's "Content" and
  "Global Constraints" — no coupling mechanism beyond keeping them
  consistent by hand).

- [ ] **Step 1: Add `?` to the main keybinding table**

In `README.md`, in the main keybinding table, add a row for `?` right
after the `a` row (run `grep -n '| \`a\` |' README.md` to find its exact
current line — don't hardcode a line number, it drifts as the README
changes):

```markdown
| `a` | Open the account-switch overlay |
| `?` | Open the help overlay (all keybindings) |
| `r` | Manual refresh |
```

- [ ] **Step 2: Add a "### Help overlay" subsection**

Add this subsection immediately after the existing "### Account switch"
section and before the "## Flags" section (run `grep -n '^###\|^## '
README.md` to see current section order), following the exact pattern of
the other per-panel subsections in this file:

```markdown
### Help overlay

Press `?` from anywhere — the peer table or any sub-panel — except the
search input and an active ssh session (both forward every keystroke
verbatim) to see every keybinding in the app grouped by section. Closing
it (`esc` or `?` again) returns to exactly whatever screen was open
underneath.

| Key | Action |
|---|---|
| `esc` / `?` | Close and return to the previous screen |
```

- [ ] **Step 3: Verify the README renders sensibly**

Run: `grep -n '^###' README.md` and confirm "### Help overlay" appears
in the list between "### Account switch" and "## Flags" (flags is a
top-level `##` heading, so it won't show in this grep — just confirm
"### Help overlay" is present once, after "### Account switch").

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document the ? help overlay"
```
