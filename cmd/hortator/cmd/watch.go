/*
Copyright (c) 2026 GeneClackman
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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/michael-niemand/Hortator/api/v1alpha1"
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

	m := model{
		namespace:  getNamespace(),
		allNS:      watchAllNS,
		focusTask:  watchTask,
		refreshInt: dur,
		k8sClient:  k8sClient,
		clientset:  clientset,
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
	showLogs   bool
	showDetail bool
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.logLines = nil
			}
		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.logLines = nil
			}
		case "enter":
			m.showDetail = !m.showDetail
		case "l":
			m.showLogs = !m.showLogs
			if m.showLogs && len(m.tasks) > 0 {
				return m, fetchLogs(m)
			}
			m.logLines = nil
		case "r":
			return m, fetchTasks(m)
		case "n":
			// Cycle to next namespace
			if len(m.namespaces) > 0 {
				m.nsIndex = (m.nsIndex + 1) % len(m.namespaces)
				m.namespace = m.namespaces[m.nsIndex]
				m.allNS = false
				m.cursor = 0
				m.logLines = nil
				return m, fetchTasks(m)
			}
		case "N":
			// Cycle to previous namespace
			if len(m.namespaces) > 0 {
				m.nsIndex = (m.nsIndex - 1 + len(m.namespaces)) % len(m.namespaces)
				m.namespace = m.namespaces[m.nsIndex]
				m.allNS = false
				m.cursor = 0
				m.logLines = nil
				return m, fetchTasks(m)
			}
		case "A":
			// Toggle all-namespaces
			m.allNS = !m.allNS
			m.cursor = 0
			m.logLines = nil
			return m, fetchTasks(m)
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
		if m.showLogs && len(m.tasks) > 0 {
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

	// --- Tasks ---
	maxVisible := m.height - 16
	if m.showDetail {
		maxVisible -= 8
	}
	if m.showLogs {
		maxVisible -= 8
	}
	if maxVisible < 3 {
		maxVisible = 3
	}

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
	// Inject "Tasks" and hint into top border
	taskBox = injectBorderTitle(taskBox, " Tasks ", " ↑↓ navigate ")
	sections = append(sections, taskBox)

	// --- Details ---
	if m.showDetail && m.cursor < len(m.tasks) {
		detailContent := renderDetails(m.tasks[m.cursor], contentWidth-4)
		detailBox := styleBorder.Width(contentWidth).Render(detailContent)
		detailBox = injectBorderTitle(detailBox, " Details ", "")
		sections = append(sections, detailBox)
	}

	// --- Logs ---
	if m.showLogs {
		var logContent string
		if len(m.logLines) == 0 {
			logContent = "  (no logs)"
		} else {
			var lines []string
			for _, l := range m.logLines {
				lines = append(lines, "  "+l)
			}
			logContent = strings.Join(lines, "\n")
		}
		logBox := styleBorder.Width(contentWidth).Render(logContent)
		logBox = injectBorderTitle(logBox, " Logs (tail) ", "")
		sections = append(sections, logBox)
	}

	// --- Footer ---
	footer := styleFooter.Render(fmt.Sprintf("  q quit │ ↑↓ select │ Enter details │ l logs │ n/N namespace │ A all-ns │ r refresh ─── %s", m.refreshInt))
	sections = append(sections, footer)

	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
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
