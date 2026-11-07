package main

import (
	_ "embed"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type welcomemodel struct {
	next tea.Model
}

var _ tea.Model = welcomemodel{}

func (m welcomemodel) View() tea.View {
	x := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	return tea.View{
		AltScreen:                 false,
		Content:                   x.Render("\n\n\n\n" + welcomemessage + "\n" + warningmessage + "\n\nPress ENTER to proceed, or ctrl+c to quit.\n"),
		WindowTitle:               "Welcome to Orchard AI",
		DisableBracketedPasteMode: true,
	}
}

func (m welcomemodel) Init() tea.Cmd {
	return nil
}

func (m welcomemodel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "ctrl+q":
			return m, tea.Quit
		default:
			return m.next, m.next.Init()
		}
	}
	return m, nil
}

//go:embed logo.txt
var logo string

//go:embed flags.txt
var flagstxt string

var welcomemessage = logo + "\n\n\nWelcome to Orchard AI (Beta)\n" +
	`
Copyright (c) 2026 aerth

All Rights Reserved.

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.` +
	"\n--------\nFlags:\n" + flagstxt +
	"\n--------\nWARNING!! This program is dangerous!" +
	"\n--------\nSource Code: https://github.com/aerth/orchard" +
	""

var warningmessage = "\n\n" + helpmode + "\n\n"

var helpmode = "Keybinds: ctrl+c/esc: cancel, ctrl+q: quit, shift+enter: newline, shift+tab: toggle render, enter: send"
