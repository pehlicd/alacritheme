package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pelletier/go-toml/v2"
)

type model struct {
	list         list.Model
	viewport     viewport.Model
	items        []list.Item
	themesDir    string
	windowSize   tea.WindowSizeMsg
	ready        bool
	err          error
	configFile   string
	originalToml []byte
	tomlBackup   map[string]interface{}
	lastSelected int
}

type item struct {
	title       string
	path        string
	isDirectory bool
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.path }
func (i item) FilterValue() string { return i.title }

type filesLoadedMsg struct {
	items []list.Item
	err   error
}

type themeSelectedMsg struct {
	path string
	err  error
}

// ColorScheme represents the Alacritty color configuration
type ColorScheme struct {
	Colors struct {
		Primary struct {
			Background string
			Foreground string
		}
		Normal struct {
			Black   string
			Red     string
			Green   string
			Yellow  string
			Blue    string
			Magenta string
			Cyan    string
			White   string
		}
		Bright struct {
			Black   string
			Red     string
			Green   string
			Yellow  string
			Blue    string
			Magenta string
			Cyan    string
			White   string
		}
	}
}

// renderColorBox creates a scaled colored box with a label
func renderColorBox(color, label string, boxWidth int) string {
	// Calculate sizes based on available width
	labelStyle := lipgloss.NewStyle().
		Width(boxWidth).
		Align(lipgloss.Center)

	boxStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(color)).
		Width(boxWidth).
		Height(1).
		Align(lipgloss.Center)

	return fmt.Sprintf("%s\n%s",
		boxStyle.Render(" "),
		labelStyle.Render(lipgloss.NewStyle().Width(boxWidth).Render(label)))
}

// renderColorPreview creates a dynamically scaled preview of the color scheme
func renderColorPreview(content string, viewportWidth int) string {
	var scheme ColorScheme
	if err := toml.Unmarshal([]byte(content), &scheme); err != nil {
		return fmt.Sprintf("Error parsing theme: %v", err)
	}

	// Calculate dynamic sizes based on viewport
	contentWidth := viewportWidth - 4 // Account for borders and padding
	numColumns := 4
	boxWidth := (contentWidth - (numColumns-1)*2) / numColumns // Account for spacing between boxes

	// Define colors with their labels
	normalColors := []struct {
		color string
		name  string
	}{
		{scheme.Colors.Normal.Black, "Black"},
		{scheme.Colors.Normal.Red, "Red"},
		{scheme.Colors.Normal.Green, "Green"},
		{scheme.Colors.Normal.Yellow, "Yellow"},
		{scheme.Colors.Normal.Blue, "Blue"},
		{scheme.Colors.Normal.Magenta, "Magenta"},
		{scheme.Colors.Normal.Cyan, "Cyan"},
		{scheme.Colors.Normal.White, "White"},
	}

	brightColors := []struct {
		color string
		name  string
	}{
		{scheme.Colors.Bright.Black, "Bright Black"},
		{scheme.Colors.Bright.Red, "Bright Red"},
		{scheme.Colors.Bright.Green, "Bright Green"},
		{scheme.Colors.Bright.Yellow, "Bright Yellow"},
		{scheme.Colors.Bright.Blue, "Bright Blue"},
		{scheme.Colors.Bright.Magenta, "Bright Magenta"},
		{scheme.Colors.Bright.Cyan, "Bright Cyan"},
		{scheme.Colors.Bright.White, "Bright White"},
	}

	// Render background/foreground section
	bgfgStyle := lipgloss.NewStyle().
		Width(contentWidth).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("69"))

	bgfg := bgfgStyle.Render(
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			renderColorBox(scheme.Colors.Primary.Background, "Background", boxWidth),
			strings.Repeat(" ", 2),
			renderColorBox(scheme.Colors.Primary.Foreground, "Foreground", boxWidth),
		),
	)

	// Render color groups (normal and bright)
	renderColorGroup := func(colors []struct{ color, name string }, title string) string {
		var rows []string
		for i := 0; i < len(colors); i += numColumns {
			end := i + numColumns
			if end > len(colors) {
				end = len(colors)
			}

			row := make([]string, 0, numColumns)
			for _, c := range colors[i:end] {
				row = append(row, renderColorBox(c.color, c.name, boxWidth))
			}
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, row...))
		}

		return lipgloss.NewStyle().
			Width(contentWidth).
			Padding(1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("69")).
			Render(lipgloss.JoinVertical(lipgloss.Center,
				rows...,
			))
	}

	normal := renderColorGroup(normalColors, "Normal Colors")
	bright := renderColorGroup(brightColors, "Bright Colors")

	// Create title style
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Background(lipgloss.NoColor{}).
		PaddingTop(1).
		Width(contentWidth).
		Align(lipgloss.Center)

	// Join all sections with proper spacing
	return lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.Render("Theme Preview"),
		"",
		titleStyle.Render("Background/Foreground Colors"),
		bgfg,
		"",
		titleStyle.Render("Normal Colors"),
		normal,
		"",
		titleStyle.Render("Bright Colors"),
		bright,
	)
}

func initialModel() model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Alacritheme"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Background(lipgloss.NoColor{}).PaddingTop(1)

	return model{
		list:         l,
		themesDir:    os.Getenv("THEMES_DIR"),
		ready:        false,
		configFile:   os.Getenv("CONFIG_FILE"),
		tomlBackup:   make(map[string]interface{}),
		lastSelected: -1,
	}
}

func (m model) Init() tea.Cmd {
	// check if the config file exists
	if _, err := os.Stat(m.configFile); os.IsNotExist(err) {
		// create the config file
		if _, err := os.Create(m.configFile); err != nil {
			m.err = err
			return nil
		}
	}

	return loadFiles(m.themesDir)
}

func loadFiles(dir string) tea.Cmd {
	return func() tea.Msg {
		var items []list.Item

		files, err := os.ReadDir(dir)
		if err != nil {
			return filesLoadedMsg{nil, err}
		}

		// Add parent directory entry except for the initial themes directory
		if dir != os.Getenv("THEMES_DIR") {
			items = append(items, item{
				title:       "..",
				path:        filepath.Dir(dir),
				isDirectory: true,
			})
		}

		for _, file := range files {
			filePath := filepath.Join(dir, file.Name())
			if file.IsDir() || strings.HasSuffix(file.Name(), ".toml") {
				items = append(items, item{
					title:       file.Name(),
					path:        filePath,
					isDirectory: file.IsDir(),
				})
			}
		}

		return filesLoadedMsg{items, nil}
	}
}

func (m *model) backupConfig() error {
	content, err := os.ReadFile(m.configFile)
	if err != nil {
		return err
	}

	var config map[string]interface{}
	if err := toml.Unmarshal(content, &config); err != nil {
		return err
	}

	m.originalToml = content
	m.tomlBackup = config
	return err
}

func (m *model) updateConfig(selectedPath string) error {
	content, err := os.ReadFile(m.configFile)
	if err != nil {
		return err
	}

	var config map[string]interface{}
	if err := toml.Unmarshal(content, &config); err != nil {
		return err
	}

	if config == nil {
		config = make(map[string]interface{})
	}

	if general, ok := config["general"].(map[string]interface{}); ok {
		general["live_config_reload"] = true
		general["import"] = []string{selectedPath}
	} else {
		config["live_config_reload"] = true
		config["import"] = []string{selectedPath}
	}

	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	encoder.SetIndentTables(true)
	if err := encoder.Encode(config); err != nil {
		return err
	}

	return os.WriteFile(m.configFile, buf.Bytes(), 0644)
}

func (m *model) restoreConfig() error {
	return os.WriteFile(m.configFile, m.originalToml, 0644)
}

// Update the model's handleSelection method to pass viewport dimensions
func (m *model) handleSelection() tea.Cmd {
	currentIndex := m.list.Index()

	if m.lastSelected == currentIndex && m.lastSelected != -1 {
		return nil
	}

	m.lastSelected = currentIndex
	if i, ok := m.list.SelectedItem().(item); ok {
		if !i.isDirectory && strings.HasSuffix(i.path, ".toml") {
			content, err := os.ReadFile(i.path)
			if err != nil {
				m.err = err
				return nil
			}

			// Pass viewport dimensions to renderColorPreview
			preview := renderColorPreview(string(content), m.viewport.Width)
			m.viewport.SetContent(preview)

			return func() tea.Msg {
				if err := m.updateConfig(i.path); err != nil {
					return themeSelectedMsg{path: i.path, err: err}
				}
				return themeSelectedMsg{path: i.path, err: nil}
			}
		}
	}

	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowSize = msg
		if !m.ready {
			m.viewport = viewport.New(msg.Width/2, msg.Height)
			m.list.SetWidth(msg.Width / 2)
			m.list.SetHeight(msg.Height)
			m.ready = true
		}

	case filesLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		// Set the full list of items (unfiltered)
		m.items = msg.items
		m.list.SetItems(m.items)

		// Handle initial selection for the first item
		cmds = append(cmds, m.handleSelection())

	case tea.KeyMsg:
		switch msg.String() {
		case tea.KeyCtrlC.String(), "q":
			if err := m.restoreConfig(); err != nil {
				m.err = err
			}
			return m, tea.Quit
		case tea.KeyEnter.String():
			newList, cmd := m.list.Update(msg)
			m.list = newList
			cmds = append(cmds, cmd)

			cmds = append(cmds, m.handleSelection())
			return m, tea.Quit
		case tea.KeyUp.String(), tea.KeyDown.String(), "k", "j":
			newList, cmd := m.list.Update(msg)
			m.list = newList
			cmds = append(cmds, cmd)

			cmds = append(cmds, m.handleSelection())
		case tea.KeyRight.String(), tea.KeyPgDown.String(), "l":
			m.list.NextPage()
			cmds = append(cmds, m.handleSelection())
		case tea.KeyLeft.String(), tea.KeyPgUp.String(), "h":
			m.list.PrevPage()
			cmds = append(cmds, m.handleSelection())
		case "/": // Add explicit filter trigger
			m.list.ShowFilter()
			return m, nil
		default:
			newList, cmd := m.list.Update(msg)
			m.list = newList
			cmds = append(cmds, cmd)
		}
	}

	// Handle viewport updates
	newViewport, cmd := m.viewport.Update(msg)
	m.viewport = newViewport
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		m.list.View(),
		m.viewport.View(),
	)
}

func main() {
	m := initialModel()
	if err := m.backupConfig(); err != nil {
		fmt.Printf("error: couldn't backup config")
		os.Exit(1)
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
