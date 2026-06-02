package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type stubScreen struct{ name string }

func (s stubScreen) Init() tea.Cmd                    { return nil }
func (s stubScreen) Update(tea.Msg) (screen, tea.Cmd) { return s, nil }
func (s stubScreen) View() string                     { return s.name }

func TestAppPushPop(t *testing.T) {
	var a tea.Model = NewApp(stubScreen{"root"})

	if got := a.View(); got != "root" {
		t.Fatalf("initial view = %q, want root", got)
	}

	a, _ = a.Update(pushScreen{stubScreen{"child"}})
	if got := a.View(); got != "child" {
		t.Fatalf("after push view = %q, want child", got)
	}

	a, _ = a.Update(popScreen{})
	if got := a.View(); got != "root" {
		t.Fatalf("after pop view = %q, want root", got)
	}

	// pop 在 root 應維持 root（不會彈空）
	a, _ = a.Update(popScreen{})
	if got := a.View(); got != "root" {
		t.Fatalf("pop at root view = %q, want root", got)
	}
}
