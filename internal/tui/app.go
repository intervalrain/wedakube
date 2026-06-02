package tui

import tea "github.com/charmbracelet/bubbletea"

// screen 是畫面堆疊裡的一層。回傳更新後的自己 + command。
type screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
}

// pushScreen / popScreen 是導覽用的 Msg：往下推一層 / 退回上一層。
type pushScreen struct{ s screen }
type popScreen struct{}

func push(s screen) tea.Cmd { return func() tea.Msg { return pushScreen{s} } }
func pop() tea.Cmd          { return func() tea.Msg { return popScreen{} } }

// App 是 root model，維護畫面堆疊並轉發訊息給最上層。
type App struct {
	stack []screen
}

func NewApp(root screen) App {
	return App{stack: []screen{root}}
}

func (a App) Init() tea.Cmd {
	return a.top().Init()
}

func (a App) top() screen { return a.stack[len(a.stack)-1] }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		return a, tea.Quit
	}

	switch m := msg.(type) {
	case pushScreen:
		a.stack = append(a.stack, m.s)
		return a, a.top().Init()
	case popScreen:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		return a, nil
	}

	i := len(a.stack) - 1
	updated, cmd := a.stack[i].Update(msg)
	a.stack[i] = updated
	return a, cmd
}

func (a App) View() string {
	return a.top().View()
}
