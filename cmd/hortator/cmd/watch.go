/*
Copyright (c) 2026 hortator-ai
SPDX-License-Identifier: MIT
*/

package cmd

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/hortator-ai/Hortator/api/v1alpha1"
)

var (
	watchRefresh string
	watchTask    string
	watchAllNS   bool
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Live TUI dashboard of agent tasks",
	Long: `Launch a full-screen terminal UI showing a live, auto-refreshing
dashboard of agent tasks with tree hierarchy, details, and logs.

Examples:
  hortator watch
  hortator watch --refresh 5s
  hortator watch --task fix-api
  hortator watch -A`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().StringVarP(&watchRefresh, "refresh", "r", "2s", "Refresh interval (e.g. 2s, 5s)")
	watchCmd.Flags().StringVarP(&watchTask, "task", "t", "", "Focus on a specific task and its children")
	watchCmd.Flags().BoolVarP(&watchAllNS, "all-namespaces", "A", false, "Watch all namespaces")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	dur, err := time.ParseDuration(watchRefresh)
	if err != nil {
		return fmt.Errorf("invalid refresh interval: %w", err)
	}

	ti := textinput.New()
	ti.Placeholder = "namespace..."
	ti.CharLimit = 63

	m := model{
		namespace:  getNamespace(),
		allNS:      watchAllNS,
		focusTask:  watchTask,
		refreshInt: dur,
		k8sClient:  k8sClient,
		clientset:  clientset,
		nsInput:    ti,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// --- Messages ---

type tickMsg struct{}
type tasksMsg struct {
	items []taskItem
	err   error
}
type logsMsg struct {
	lines []string
	err   error
}
type namespacesMsg struct {
	items []string
	err   error
}

// --- View mode ---

type viewMode int

const (
	viewList     viewMode = iota // task list (default)
	viewDetails                  // quick details
	viewDescribe                 // full describe (spec + output)
	viewLogs                     // pod logs
	viewSummary                  // aggregate summary
)

// --- Model ---

type model struct {
	tasks      []taskItem
	cursor     int
	width      int
	height     int
	namespace  string
	namespaces []string // discovered namespaces for cycling
	nsIndex    int      // current index in namespaces slice
	allNS      bool
	focusTask  string
	refreshInt time.Duration
	k8sClient  client.Client
	clientset  *kubernetes.Clientset
	lastErr    error
	logLines   []string

	// Current view mode (replaces showLogs/showDetail/showDescribe/showSummary)
	view viewMode

	// Namespace text input mode
	nsInput     textinput.Model
	nsInputMode bool

	// Scroll offset for content views
	scrollOffset int
}

type taskItem struct {
	task   corev1alpha1.AgentTask
	depth  int
	prefix string
}

// --- Logo ---

const hortatorLogo = `  ██╗  ██╗ ██████╗ ██████╗ ████████╗ █████╗ ████████╗ ██████╗ ██████╗
  ██║  ██║██╔═══██╗██╔══██╗╚══██╔══╝██╔══██╗╚══██╔══╝██╔═══██╗██╔══██╗
  ███████║██║   ██║██████╔╝   ██║   ███████║   ██║   ██║   ██║██████╔╝
  ██╔══██║██║   ██║██╔══██╗   ██║   ██╔══██║   ██║   ██║   ██║██╔══██╗
  ██║  ██║╚██████╔╝██║  ██║   ██║   ██║  ██║   ██║   ╚██████╔╝██║  ██║
  ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝
                        Remigate, vermēs!`

// --- Styles ---

var (
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).MarginLeft(1)
	styleSubtle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleFooter = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	styleRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))  // yellow
	styleCompleted = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))  // green
	styleFailed    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // red
	stylePending   = lipgloss.NewStyle().Foreground(lipgloss.Color("245")) // gray
	styleRetrying  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))  // cyan

	styleTribune   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")) // magenta
	styleCenturion = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4")) // blue
	styleLegionary = lipgloss.NewStyle().Faint(true)

	styleSelected = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("15"))
	styleCostOk   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleCostHigh = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99"))

	styleLogo = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
)

// --- Tea interface ---

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchTasks(m), fetchNamespaces(m), tick(m.refreshInt))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If namespace input mode is active, delegate to text input
	if m.nsInputMode {
		return m.updateNsInput(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// In any sub-view, Esc returns to list
		if m.view != viewList && msg.String() == "esc" {
			m.view = viewList
			m.scrollOffset = 0
			m.logLines = nil
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.view == viewList {
				if m.cursor > 0 {
					m.cursor--
				}
			} else {
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
			}
		case "down", "j":
			if m.view == viewList {
				if m.cursor < len(m.tasks)-1 {
					m.cursor++
				}
			} else {
				m.scrollOffset++
			}
		case "enter", "d":
			if m.view == viewList {
				m.view = viewDetails
				m.scrollOffset = 0
			} else {
				m.view = viewList
				m.scrollOffset = 0
			}
		case "l":
			if m.view == viewList {
				m.view = viewLogs
				m.scrollOffset = 0
				m.logLines = nil
				if len(m.tasks) > 0 {
					return m, fetchLogs(m)
				}
			} else {
				m.view = viewList
				m.scrollOffset = 0
			}
		case "D":
			if m.view == viewList {
				m.view = viewDescribe
				m.scrollOffset = 0
			} else {
				m.view = viewList
				m.scrollOffset = 0
			}
		case "s", "S":
			if m.view == viewList {
				m.view = viewSummary
				m.scrollOffset = 0
			} else {
				m.view = viewList
				m.scrollOffset = 0
			}
		case "r":
			return m, fetchTasks(m)
		case "n":
			if m.view == viewList {
				m.nsInputMode = true
				m.nsInput.SetValue(m.namespace)
				m.nsInput.Focus()
				return m, nil
			}
		case "N":
			if m.view == viewList && len(m.namespaces) > 0 {
				m.nsIndex = (m.nsIndex - 1 + len(m.namespaces)) % len(m.namespaces)
				m.namespace = m.namespaces[m.nsIndex]
				m.allNS = false
				m.cursor = 0
				return m, fetchTasks(m)
			}
		case "A":
			if m.view == viewList {
				m.allNS = !m.allNS
				m.cursor = 0
				return m, fetchTasks(m)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tea.Batch(fetchTasks(m), tick(m.refreshInt))

	case tasksMsg:
		m.lastErr = msg.err
		if msg.err == nil {
			m.tasks = msg.items
			if m.cursor >= len(m.tasks) && len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
			}
		}
		if m.view == viewLogs && len(m.tasks) > 0 {
			return m, fetchLogs(m)
		}

	case logsMsg:
		if msg.err == nil {
			m.logLines = msg.lines
		}

	case namespacesMsg:
		if msg.err == nil {
			m.namespaces = msg.items
			// Set nsIndex to current namespace
			for i, ns := range m.namespaces {
				if ns == m.namespace {
					m.nsIndex = i
					break
				}
			}
		}
	}

	return m, nil
}

// updateNsInput handles input while namespace text input is active.
func (m model) updateNsInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Accept the input
			ns := strings.TrimSpace(m.nsInput.Value())
			if ns != "" {
				m.namespace = ns
				m.allNS = false
				m.cursor = 0
				m.logLines = nil
			}
			m.nsInputMode = false
			m.nsInput.Blur()
			return m, fetchTasks(m)
		case "esc":
			// Cancel
			m.nsInputMode = false
			m.nsInput.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.nsInput, cmd = m.nsInput.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	contentWidth := m.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	var sections []string

	// --- Header ---
	nsLabel := m.namespace
	if m.allNS {
		nsLabel = "all"
	}
	logo := styleLogo.Render(hortatorLogo)
	nsLine := styleSubtle.Render(fmt.Sprintf("                        namespace: %s", nsLabel))
	headerContent := lipgloss.JoinVertical(lipgloss.Left, logo, nsLine)
	headerBox := styleBorder.Width(contentWidth).Render(headerContent)
	sections = append(sections, headerBox)

	// --- Error ---
	if m.lastErr != nil {
		errBox := styleBorder.
			Width(contentWidth).
			BorderForeground(lipgloss.Color("9")).
			Render(fmt.Sprintf("  Error: %v", m.lastErr))
		sections = append(sections, errBox)
	}

	// --- Namespace Input (overlays any view) ---
	if m.nsInputMode {
		inputContent := fmt.Sprintf("  Namespace: %s", m.nsInput.View())
		inputBox := styleBorder.Width(contentWidth).
			BorderForeground(lipgloss.Color("11")).
			Render(inputContent)
		inputBox = injectBorderTitle(inputBox, " Set Namespace ", " Enter confirm │ Esc cancel ")
		sections = append(sections, inputBox)
	}

	// Available lines for main content (header ~10 lines, footer 1, borders 2)
	availableLines := m.height - 14
	if m.lastErr != nil {
		availableLines -= 3
	}
	if m.nsInputMode {
		availableLines -= 3
	}
	if availableLines < 5 {
		availableLines = 5
	}

	// --- Main content area (single pane) ---
	switch m.view {
	case viewList:
		sections = append(sections, m.renderTaskListPane(contentWidth, availableLines))
	case viewDetails:
		sections = append(sections, m.renderSubView(contentWidth, availableLines, " Details ", " Esc back │ ↑↓ scroll ",
			func() string {
				if m.cursor < len(m.tasks) {
					return renderDetails(m.tasks[m.cursor], contentWidth-4)
				}
				return "  (no task selected)"
			}))
	case viewDescribe:
		sections = append(sections, m.renderSubView(contentWidth, availableLines, " Describe ", " Esc back │ ↑↓ scroll ",
			func() string {
				if m.cursor < len(m.tasks) {
					return renderDescribe(m.tasks[m.cursor], contentWidth-4)
				}
				return "  (no task selected)"
			}))
	case viewLogs:
		sections = append(sections, m.renderSubView(contentWidth, availableLines, " Logs ", " Esc back │ ↑↓ scroll ",
			func() string {
				if len(m.logLines) == 0 {
					return "  (no logs)"
				}
				var lines []string
				for _, l := range m.logLines {
					lines = append(lines, "  "+l)
				}
				return strings.Join(lines, "\n")
			}))
	case viewSummary:
		sections = append(sections, m.renderSubView(contentWidth, availableLines, " Summary ", " Esc back │ ↑↓ scroll ",
			func() string {
				return renderSummary(m.tasks, contentWidth-4)
			}))
	}

	// --- Footer ---
	var footer string
	if m.view == viewList {
		footer = styleFooter.Render(fmt.Sprintf("  q quit │ ↑↓ select │ d details │ D describe │ s summary │ l logs │ n namespace │ A all-ns │ r refresh ─── %s", m.refreshInt))
	} else {
		taskName := ""
		if m.cursor < len(m.tasks) {
			taskName = m.tasks[m.cursor].task.Name
		}
		footer = styleFooter.Render(fmt.Sprintf("  Esc back │ ↑↓ scroll │ q quit ─── %s", taskName))
	}
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

// renderTaskListPane renders the task list as the main content pane.
func (m model) renderTaskListPane(contentWidth, maxVisible int) string {
	var taskLines []string
	if len(m.tasks) == 0 {
		taskLines = append(taskLines, "  No tasks found.")
	} else {
		for i, item := range m.tasks {
			if i >= maxVisible {
				taskLines = append(taskLines, fmt.Sprintf("  ... and %d more", len(m.tasks)-i))
				break
			}
			line := renderTaskLine(item, contentWidth-4)
			if i == m.cursor {
				line = styleSelected.Render(line)
			}
			taskLines = append(taskLines, line)
		}
	}

	taskContent := strings.Join(taskLines, "\n")
	taskBox := styleBorder.Width(contentWidth).Render(taskContent)
	taskBox = injectBorderTitle(taskBox, " Tasks ", " ↑↓ navigate ")
	return taskBox
}

// renderSubView renders a scrollable content view in place of the task list.
func (m model) renderSubView(contentWidth, maxLines int, title, hint string, contentFn func() string) string {
	content := contentFn()
	lines := strings.Split(content, "\n")

	// Clamp scroll offset
	maxOffset := len(lines) - maxLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.scrollOffset
	if offset > maxOffset {
		offset = maxOffset
	}

	// Slice visible window
	end := offset + maxLines
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[offset:end]

	// Add scroll indicator
	if offset > 0 {
		visible = append([]string{styleSubtle.Render("  ↑ more above")}, visible...)
		if len(visible) > maxLines {
			visible = visible[:maxLines]
		}
	}
	if end < len(lines) {
		if len(visible) >= maxLines {
			visible[maxLines-1] = styleSubtle.Render("  ↓ more below")
		} else {
			visible = append(visible, styleSubtle.Render("  ↓ more below"))
		}
	}

	rendered := strings.Join(visible, "\n")
	box := styleBorder.Width(contentWidth).Render(rendered)
	box = injectBorderTitle(box, title, hint)
	return box
}

// injectBorderTitle replaces part of the top border line with a title and optional right-side hint.
func injectBorderTitle(box string, title string, hint string) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	top := []rune(lines[0])
	titleRunes := []rune(styleTitle.Render(title))

	// Insert title after first 2 border chars
	if len(top) > 3 {
		result := string(top[:2]) + string(titleRunes)
		remaining := len(top) - 2 - lipgloss.Width(string(titleRunes))
		if hint != "" && remaining > len(hint)+2 {
			hintRendered := styleSubtle.Render(hint)
			hintWidth := lipgloss.Width(hintRendered)
			padding := remaining - hintWidth
			if padding > 0 {
				for i := 0; i < padding; i++ {
					result += "─"
				}
				result += hintRendered
			} else {
				for i := 0; i < remaining; i++ {
					result += "─"
				}
			}
		} else {
			if remaining > 0 {
				for i := 0; i < remaining; i++ {
					result += "─"
				}
			}
		}
		result += string(top[len(top)-1:])
		lines[0] = result
	}
	return strings.Join(lines, "\n")
}

// --- Rendering helpers ---

func phaseIcon(phase corev1alpha1.AgentTaskPhase) string {
	switch phase {
	case corev1alpha1.AgentTaskPhaseCompleted:
		return styleCompleted.Render("✓")
	case corev1alpha1.AgentTaskPhaseFailed:
		return styleFailed.Render("✗")
	case corev1alpha1.AgentTaskPhaseRunning:
		return styleRunning.Render("●")
	case corev1alpha1.AgentTaskPhaseRetrying:
		return styleRetrying.Render("◐")
	case corev1alpha1.AgentTaskPhasePending:
		return stylePending.Render("○")
	case corev1alpha1.AgentTaskPhaseCancelled:
		return stylePending.Render("⊘")
	case corev1alpha1.AgentTaskPhaseBudgetExceeded:
		return styleFailed.Render("$")
	case corev1alpha1.AgentTaskPhaseTimedOut:
		return styleFailed.Render("⏱")
	default:
		return stylePending.Render("?")
	}
}

func tierStyle(tier string) lipgloss.Style {
	switch strings.ToLower(tier) {
	case "tribune":
		return styleTribune
	case "centurion":
		return styleCenturion
	default:
		return styleLegionary
	}
}

func phaseStyle(phase corev1alpha1.AgentTaskPhase) lipgloss.Style {
	switch phase {
	case corev1alpha1.AgentTaskPhaseRunning:
		return styleRunning
	case corev1alpha1.AgentTaskPhaseCompleted:
		return styleCompleted
	case corev1alpha1.AgentTaskPhaseFailed, corev1alpha1.AgentTaskPhaseBudgetExceeded, corev1alpha1.AgentTaskPhaseTimedOut:
		return styleFailed
	case corev1alpha1.AgentTaskPhaseRetrying:
		return styleRetrying
	default:
		return stylePending
	}
}

func renderTaskLine(item taskItem, _ int) string {
	t := item.task
	icon := phaseIcon(t.Status.Phase)
	name := truncate(t.Name, 24)
	tier := tierStyle(t.Spec.Tier).Render(fmt.Sprintf("%-10s", capitalize(t.Spec.Tier)))
	phase := phaseStyle(t.Status.Phase).Render(fmt.Sprintf("%-12s", string(t.Status.Phase)))
	dur := t.Status.Duration
	if dur == "" {
		dur = elapsed(t)
	}
	cost := t.Status.EstimatedCostUsd
	if cost == "" {
		cost = "-"
	} else {
		cost = "$" + cost
	}

	indent := strings.Repeat("  ", item.depth)
	prefix := item.prefix

	return fmt.Sprintf("  %s%s%s %-24s %s %s %-8s %6s", indent, prefix, icon, name, tier, phase, dur, cost)
}

func elapsed(t corev1alpha1.AgentTask) string {
	if t.Status.StartedAt == nil {
		return "-"
	}
	end := time.Now()
	if t.Status.CompletedAt != nil {
		end = t.Status.CompletedAt.Time
	}
	d := end.Sub(t.Status.StartedAt.Time)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

func renderDetails(item taskItem, _ int) string {
	t := item.task
	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Name: %s\n", t.Name))

	maxAttempts := 1
	if t.Spec.Retry != nil && t.Spec.Retry.MaxAttempts > 0 {
		maxAttempts = t.Spec.Retry.MaxAttempts
	}
	b.WriteString(fmt.Sprintf("  Role: %-14s Tier: %-12s Attempts: %d/%d\n",
		t.Spec.Role, capitalize(t.Spec.Tier), t.Status.Attempts, maxAttempts))

	tokIn, tokOut := int64(0), int64(0)
	if t.Status.TokensUsed != nil {
		tokIn = t.Status.TokensUsed.Input
		tokOut = t.Status.TokensUsed.Output
	}
	cost := t.Status.EstimatedCostUsd
	if cost == "" {
		cost = "0.00"
	}
	costStr := "$" + cost

	// Color cost based on budget
	if t.Spec.Budget != nil && t.Spec.Budget.MaxCostUsd != "" {
		maxCost, err1 := strconv.ParseFloat(t.Spec.Budget.MaxCostUsd, 64)
		curCost, err2 := strconv.ParseFloat(cost, 64)
		if err1 == nil && err2 == nil && maxCost > 0 {
			if curCost/maxCost > 0.8 {
				costStr = styleCostHigh.Render(costStr)
			} else {
				costStr = styleCostOk.Render(costStr)
			}
		}
	}

	b.WriteString(fmt.Sprintf("  Tokens: %s in / %s out    Cost: %s\n",
		formatInt(tokIn), formatInt(tokOut), costStr))

	started := "-"
	if t.Status.StartedAt != nil {
		started = t.Status.StartedAt.Format("15:04:05")
	}
	dur := t.Status.Duration
	if dur == "" {
		dur = elapsed(t)
	}
	b.WriteString(fmt.Sprintf("  Started: %s    Elapsed: %s\n", started, dur))

	if len(t.Spec.Capabilities) > 0 {
		b.WriteString(fmt.Sprintf("  Capabilities: [%s]\n", strings.Join(t.Spec.Capabilities, ", ")))
	}

	return b.String()
}

func renderDescribe(item taskItem, maxWidth int) string {
	t := item.task
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  Name:      %s\n", t.Name))
	b.WriteString(fmt.Sprintf("  Namespace: %s\n", t.Namespace))
	b.WriteString(fmt.Sprintf("  Tier:      %s\n", capitalize(t.Spec.Tier)))
	b.WriteString(fmt.Sprintf("  Role:      %s\n", t.Spec.Role))
	b.WriteString(fmt.Sprintf("  Phase:     %s\n", string(t.Status.Phase)))

	if len(t.Spec.Capabilities) > 0 {
		b.WriteString(fmt.Sprintf("  Caps:      [%s]\n", strings.Join(t.Spec.Capabilities, ", ")))
	}
	if t.Spec.Model != nil && t.Spec.Model.Name != "" {
		b.WriteString(fmt.Sprintf("  Model:     %s\n", t.Spec.Model.Name))
	}
	if t.Spec.Budget != nil {
		budgetParts := []string{}
		if t.Spec.Budget.MaxTokens != nil {
			budgetParts = append(budgetParts, fmt.Sprintf("tokens=%d", *t.Spec.Budget.MaxTokens))
		}
		if t.Spec.Budget.MaxCostUsd != "" {
			budgetParts = append(budgetParts, fmt.Sprintf("cost=$%s", t.Spec.Budget.MaxCostUsd))
		}
		if len(budgetParts) > 0 {
			b.WriteString(fmt.Sprintf("  Budget:    %s\n", strings.Join(budgetParts, ", ")))
		}
	}
	if t.Spec.ParentTaskID != "" {
		b.WriteString(fmt.Sprintf("  Parent:    %s\n", t.Spec.ParentTaskID))
	}

	// Prompt
	b.WriteString("\n")
	prompt := t.Spec.Prompt
	if len(prompt) > maxWidth*4 {
		prompt = prompt[:maxWidth*4] + "..."
	}
	b.WriteString("  ── Prompt ──\n")
	for _, line := range strings.Split(prompt, "\n") {
		b.WriteString("  " + line + "\n")
	}

	// Output (for completed/failed tasks)
	if t.Status.Output != "" {
		b.WriteString("\n  ── Output ──\n")
		output := t.Status.Output
		if len(output) > maxWidth*6 {
			output = output[:maxWidth*6] + "\n  ...(truncated)"
		}
		for _, line := range strings.Split(output, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}

	// Message
	if t.Status.Message != "" {
		b.WriteString(fmt.Sprintf("\n  Message: %s\n", t.Status.Message))
	}

	return b.String()
}

func renderSummary(items []taskItem, _ int) string {
	if len(items) == 0 {
		return "  No tasks."
	}

	phaseCounts := make(map[corev1alpha1.AgentTaskPhase]int)
	tierCounts := make(map[string]int)
	totalCost := 0.0
	totalTokensIn := int64(0)
	totalTokensOut := int64(0)

	for _, item := range items {
		t := item.task
		phaseCounts[t.Status.Phase]++
		tierCounts[capitalize(t.Spec.Tier)]++

		if t.Status.TokensUsed != nil {
			totalTokensIn += t.Status.TokensUsed.Input
			totalTokensOut += t.Status.TokensUsed.Output
		}
		if t.Status.EstimatedCostUsd != "" {
			if c, err := strconv.ParseFloat(t.Status.EstimatedCostUsd, 64); err == nil {
				totalCost += c
			}
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  Total Tasks: %d\n\n", len(items)))

	// Phase breakdown
	b.WriteString("  By Phase:\n")
	phases := []corev1alpha1.AgentTaskPhase{
		corev1alpha1.AgentTaskPhaseRunning,
		corev1alpha1.AgentTaskPhaseWaiting,
		corev1alpha1.AgentTaskPhasePending,
		corev1alpha1.AgentTaskPhaseRetrying,
		corev1alpha1.AgentTaskPhaseCompleted,
		corev1alpha1.AgentTaskPhaseFailed,
		corev1alpha1.AgentTaskPhaseBudgetExceeded,
		corev1alpha1.AgentTaskPhaseTimedOut,
		corev1alpha1.AgentTaskPhaseCancelled,
	}
	for _, phase := range phases {
		if count := phaseCounts[phase]; count > 0 {
			icon := phaseIcon(phase)
			b.WriteString(fmt.Sprintf("    %s %-16s %d\n", icon, string(phase), count))
		}
	}
	// Handle empty phase
	if count := phaseCounts[""]; count > 0 {
		b.WriteString(fmt.Sprintf("    ? %-16s %d\n", "(unknown)", count))
	}

	// Tier breakdown
	b.WriteString("\n  By Tier:\n")
	for _, tier := range []string{"Tribune", "Centurion", "Legionary"} {
		if count := tierCounts[tier]; count > 0 {
			b.WriteString(fmt.Sprintf("    %-12s %d\n", tier, count))
		}
	}

	// Totals
	b.WriteString(fmt.Sprintf("\n  Tokens: %s in / %s out\n",
		formatInt(totalTokensIn), formatInt(totalTokensOut)))
	b.WriteString(fmt.Sprintf("  Cost:   $%.4f\n", totalCost))

	return b.String()
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	// Simple comma formatting
	parts := []string{}
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

// --- Commands ---

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func fetchTasks(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		taskList := &corev1alpha1.AgentTaskList{}
		opts := []client.ListOption{}
		if !m.allNS {
			opts = append(opts, client.InNamespace(m.namespace))
		}
		if err := m.k8sClient.List(ctx, taskList, opts...); err != nil {
			return tasksMsg{err: err}
		}

		// Build parent→children map
		byName := make(map[string]*corev1alpha1.AgentTask)
		childMap := make(map[string][]string)
		roots := []string{}

		for i := range taskList.Items {
			t := &taskList.Items[i]
			byName[t.Name] = t
			if t.Spec.ParentTaskID != "" {
				childMap[t.Spec.ParentTaskID] = append(childMap[t.Spec.ParentTaskID], t.Name)
			} else {
				roots = append(roots, t.Name)
			}
		}

		// If focusing on a specific task
		if m.focusTask != "" {
			roots = []string{m.focusTask}
		}

		// Flatten tree
		var items []taskItem
		for _, name := range roots {
			t, ok := byName[name]
			if !ok {
				continue
			}
			flattenTree(t, byName, childMap, 0, "", &items)
		}

		return tasksMsg{items: items}
	}
}

func flattenTree(task *corev1alpha1.AgentTask, byName map[string]*corev1alpha1.AgentTask, childMap map[string][]string, depth int, prefix string, out *[]taskItem) {
	*out = append(*out, taskItem{task: *task, depth: depth, prefix: prefix})

	children := childMap[task.Name]
	for i, childName := range children {
		child, ok := byName[childName]
		if !ok {
			continue
		}
		isLast := i == len(children)-1
		var connector, nextPrefix string
		if depth == 0 {
			// First level children
			if isLast {
				connector = "└─ "
				nextPrefix = "   "
			} else {
				connector = "├─ "
				nextPrefix = "│  "
			}
		} else {
			if isLast {
				connector = "└─ "
				nextPrefix = "   "
			} else {
				connector = "├─ "
				nextPrefix = "│  "
			}
		}
		flattenTree(child, byName, childMap, depth+1, connector, out)
		_ = nextPrefix // prefix propagation simplified for v1
	}
}

func fetchNamespaces(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		nsList, err := m.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			return namespacesMsg{err: err}
		}

		var names []string
		for _, ns := range nsList.Items {
			names = append(names, ns.Name)
		}
		return namespacesMsg{items: names}
	}
}

func fetchLogs(m model) tea.Cmd {
	return func() tea.Msg {
		if m.cursor >= len(m.tasks) {
			return logsMsg{}
		}
		task := m.tasks[m.cursor].task
		if task.Status.PodName == "" {
			return logsMsg{lines: []string{"(no pod assigned)"}}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ns := task.Namespace
		if ns == "" {
			ns = m.namespace
		}

		tailLines := int64(20)
		opts := &corev1.PodLogOptions{
			Container: "agent",
			TailLines: &tailLines,
		}

		stream, err := m.clientset.CoreV1().Pods(ns).GetLogs(task.Status.PodName, opts).Stream(ctx)
		if err != nil {
			return logsMsg{lines: []string{fmt.Sprintf("(error: %v)", err)}}
		}
		defer func() { _ = stream.Close() }()

		var lines []string
		scanner := bufio.NewScanner(stream)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		return logsMsg{lines: lines}
	}
}
