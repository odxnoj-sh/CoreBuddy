package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// %[1]d = Core ID, %[2]d = Load Pct
var idleDialogTemplates = []string{
	"(yawns)... Load is only %[2]d%%. Wake me up when you compile a kernel.",
	"Core %[1]d resting. It's too quiet in here.",
	"I'm sleepy, but I can still feel %[2]d%% load drifting through my circuits.",
}

var activeDialogTemplates = []string{
	"Core %[1]d on duty! Processing at %[2]d%%.",
	"I'm working on it! Don't rush me.",
	"The load is %[2]d%% and I'm holding the line for core %[1]d.",
}

var overloadDialogTemplates = []string{
	"AHHH! %[2]d%% load! I'm melting!",
	"Core %[1]d is screaming! Kill the heaviest process!",
	"The load is %[2]d%% and I'm about to burst through the ceiling!",
}

type Model struct {
	width          int
	height         int
	hardware       HardwareInfo
	cores          []CoreState
	selectedCore   int
	dialogText     string
	lastCPUStats   []cpuSample
	lastMetricsAt  time.Time
	lastRenderAt   time.Time
	frame          int
	memoryUsagePct float64
}

type HardwareInfo struct {
	Vendor            string
	Model             string
	Microarchitecture string
	BaseFrequencyMHz  string
	MaxFrequencyMHz   string
	TotalCores        int
	TotalThreads      int
}

type CoreState struct {
	Index         int
	Utilization   float64
	State         string
	FrameIndex    int
	ReactionUntil time.Time
	LastProcess   string
}

type cpuSample struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
}

type TickMsg struct{}

func main() {
	program := tea.NewProgram(NewModel(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "corebuddy exited: %v\n", err)
		os.Exit(1)
	}
}

func NewModel() Model {
	info := readHardwareInfo()
	coreCount := detectCPUCount()
	cores := make([]CoreState, 0, coreCount)
	for i := 0; i < coreCount; i++ {
		cores = append(cores, CoreState{Index: i, State: "idle", LastProcess: "telemetry"})
	}
	return Model{width: 120, height: 40, hardware: info, cores: cores}
}

func (m Model) Init() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case TickMsg:
		m.frame++
		if m.lastMetricsAt.IsZero() || time.Since(m.lastMetricsAt) >= time.Second {
			m = m.advanceMetrics()
			m.lastMetricsAt = time.Now()
		}
		m = m.advanceAnimations()
		return m, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
			return TickMsg{}
		})
	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft {
			m = m.handleMouse(msg)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) View() string {
	leftWidth := 46
	if m.width < 85 {
		leftWidth = 38
	}

	rightWidth := m.width - leftWidth - 1
	if rightWidth < 24 {
		rightWidth = 24
	}

	leftPanel := m.renderLeftPanel(leftWidth)
	
	// Sağ panelin hündürlüyünü sol panelə bərabər edirik ki, çərçivələr qırılmasın
	leftHeight := lipgloss.Height(leftPanel)
	rightPanel := m.renderRightPanel(rightWidth, leftHeight)
	
	topSection := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	actualWidth := lipgloss.Width(topSection)
	bottomPanel := m.renderBottomPanel(actualWidth)

	full := lipgloss.JoinVertical(lipgloss.Left, topSection, bottomPanel)
	return full
}

func (m Model) renderLeftPanel(width int) string {
	logoLines := m.hardware.logoLines()
	lines := make([]string, 0, len(logoLines)+16)
	lines = append(lines, logoLines...)
	lines = append(lines, "")

	limitStr := func(label, val string) string {
		maxLen := width - 6
		fullLine := label + ": " + val
		if len(fullLine) > maxLen {
			allowedValLen := maxLen - len(label) - 2
			if allowedValLen > 3 {
				val = val[:allowedValLen-3] + "..."
			} else {
				return fullLine[:maxLen]
			}
		}
		return m.styleLabel(label) + ": " + val
	}

	lines = append(lines, limitStr("Vendor", m.hardware.Vendor))
	lines = append(lines, limitStr("Model", m.hardware.Model))
	lines = append(lines, limitStr("Microarch", m.hardware.Microarchitecture))
	lines = append(lines, limitStr("Base", m.hardware.BaseFrequencyMHz))
	lines = append(lines, limitStr("Max", m.hardware.MaxFrequencyMHz))
	lines = append(lines, limitStr("Cores", strconv.Itoa(m.hardware.TotalCores)))
	lines = append(lines, limitStr("Threads", strconv.Itoa(m.hardware.TotalThreads)))
	lines = append(lines, "")
	lines = append(lines, m.styleLabel("RAM")+": "+fmt.Sprintf("%.0f%%", m.memoryUsagePct))
	lines = append(lines, renderMemoryBar(int(width-6), int(m.memoryUsagePct)))

	content := strings.Join(lines, "\n")
	style := lipgloss.NewStyle().Width(width).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#cba6f7"))
	return style.Render(content)
}

func (m Model) renderRightPanel(width, height int) string {
	cardWidth := 18
	innerWidth := width - 4
	cols := innerWidth / cardWidth
	if cols < 1 {
		cols = 1
	}
	if cols > 4 {
		cols = 4
	}

	rows := max(1, (len(m.cores)+cols-1)/cols)
	rowBlocks := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		parts := make([]string, 0, cols)
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(m.cores) {
				break
			}
			parts = append(parts, m.renderCoreCard(m.cores[idx], cardWidth))
		}
		rowBlocks = append(rowBlocks, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	content := lipgloss.JoinVertical(lipgloss.Left, rowBlocks...)
	style := lipgloss.NewStyle().Width(width).Height(height).Padding(1, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#94e2d5"))
	return style.Render(content)
}

func (m Model) renderCoreCard(core CoreState, width int) string {
	face := core.face()
	load := fmt.Sprintf("Load: %.0f%%", core.Utilization)
	state := strings.ToUpper(core.State)

	var stateColor, faceColor string
	switch core.State {
	case "active":
		stateColor = "#a6e3a1"
		faceColor = "#a6e3a1"
	case "overload":
		stateColor = "#f38ba8"
		faceColor = "#f38ba8"
	default:
		stateColor = "#89b4fa"
		faceColor = "#89b4fa"
	}

	if !core.ReactionUntil.IsZero() && time.Now().Before(core.ReactionUntil) {
		faceColor = "#f9e2af"
	}

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f5c2e7")).Render(fmt.Sprintf("Core %d", core.Index+1))
	loadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Render(load)
	stateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(stateColor)).Render(state)
	faceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(faceColor)).Render(face)

	block := strings.Join([]string{label, loadStyle, faceStyle, stateStyle}, "\n")
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Align(lipgloss.Center).Render(block)
}

func (m Model) renderBottomPanel(width int) string {
	content := m.dialogText
	if content == "" {
		content = "Click any core to trigger a biological reaction and inspect its telemetry."
	}
	style := lipgloss.NewStyle().Width(width).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f38ba8"))
	return style.Render(content)
}

func (m Model) handleMouse(msg tea.MouseMsg) Model {
	leftWidth := 46
	if m.width < 85 {
		leftWidth = 38
	}
	rightWidth := m.width - leftWidth - 1
	if rightWidth < 24 {
		rightWidth = 24
	}
	if msg.X <= leftWidth+1 {
		return m
	}
	cardWidth := 18
	innerWidth := rightWidth - 4
	cols := innerWidth / cardWidth
	if cols < 1 {
		cols = 1
	}
	if cols > 4 {
		cols = 4
	}

	rows := max(1, (len(m.cores)+cols-1)/cols)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(m.cores) {
				break
			}
			startX := leftWidth + 2 + col*cardWidth
			startY := 2 + row*5
			if msg.X >= startX && msg.X <= startX+cardWidth && msg.Y >= startY && msg.Y <= startY+5 {
				core := &m.cores[idx]
				core.ReactionUntil = time.Now().Add(2 * time.Second)
				m.selectedCore = core.Index
				m.dialogText = m.buildDialog(core)
				return m
			}
		}
	}
	return m
}

func (m Model) buildDialog(core *CoreState) string {
	var templates []string
	switch core.State {
	case "active":
		templates = activeDialogTemplates
	case "overload":
		templates = overloadDialogTemplates
	default:
		templates = idleDialogTemplates
	}
	template := templates[rand.Intn(len(templates))]
	load := int(core.Utilization)
	return fmt.Sprintf(template, core.Index+1, load)
}

func (m Model) advanceMetrics() Model {
	loads, samples, err := readCPUStats(m.lastCPUStats)
	if err == nil {
		m.lastCPUStats = samples
		for i := range m.cores {
			if i >= len(loads) {
				break
			}
			core := &m.cores[i]
			core.Utilization = loads[i]
			core.LastProcess = readTopProcess()
			switch {
			case core.Utilization < 15:
				core.State = "idle"
			case core.Utilization < 70:
				core.State = "active"
			default:
				core.State = "overload"
			}
			if !core.ReactionUntil.IsZero() && time.Now().After(core.ReactionUntil) {
				core.ReactionUntil = time.Time{}
			}
		}
		m.memoryUsagePct = readMemoryUsage()
	}
	return m
}

func (m Model) advanceAnimations() Model {
	for i := range m.cores {
		core := &m.cores[i]
		core.FrameIndex++
		if core.FrameIndex > 3 {
			core.FrameIndex = 0
		}
	}
	return m
}

func (c CoreState) face() string {
	if !c.ReactionUntil.IsZero() && time.Now().Before(c.ReactionUntil) {
		return "( O ▃ O )"
	}
	switch c.State {
	case "active":
		if c.FrameIndex%2 == 0 {
			return "( ಠ _ ಠ )"
		}
		return "[ ⊙ _ ⊙ ]"
	case "overload":
		if c.FrameIndex%2 == 0 {
			return "( Ò 益 Ó )"
		}
		return "[ ☠ _ ☠ ]"
	default:
		if c.FrameIndex%2 == 0 {
			return "( ˘ ▾ ˘ )"
		}
		return "[ - ˕ - ]"
	}
}

func (m Model) styleLabel(label string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f9e2af")).Render(label)
}

func (h HardwareInfo) logoLines() []string {
	if strings.Contains(strings.ToLower(h.Vendor), "amd") {
		amdLogo := `
  .============
    -===-======
    =      -=-=
  -=-      -=--
 ====      -===
 ====--===  ===
 =======      =`
		return strings.Split(strings.Trim(amdLogo, "\n"), "\n")
	}

	defaultLogo := `
       **************
     *** ****
    * **
    ** ** * **
    * ** ** **
    * ** ******* *** ****** ** **
    * ** ** ** ** ******* ** ***
    * ** ** ** ** ** ** ***
    * * ** ** ** *** * *
    **
    *** ****
      ******* ***********
          ***************`
	return strings.Split(strings.Trim(defaultLogo, "\n"), "\n")
}

func detectCPUCount() int {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 1
	}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if isCPUCoreStatLine(strings.TrimSpace(scanner.Text())) {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

func isCPUCoreStatLine(line string) bool {
	if line == "" {
		return false
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}
	name := parts[0]
	if name == "cpu" {
		return false
	}
	if !strings.HasPrefix(name, "cpu") {
		return false
	}
	if len(name) <= 3 {
		return false
	}
	for _, ch := range name[3:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func readHardwareInfo() HardwareInfo {
	info := HardwareInfo{Vendor: "Unknown", Model: "Generic CPU", Microarchitecture: "Generic", BaseFrequencyMHz: "N/A", MaxFrequencyMHz: "N/A", TotalCores: detectCPUCount(), TotalThreads: detectCPUCount()}
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return info
	}
	var model, vendor, microarch, baseMHz, maxMHz string
	processorCount := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "vendor_id":
			vendor = value
		case "model name":
			model = value
		case "cpu MHz":
			baseMHz = value
		case "cpu family":
			microarch = value
		case "processor":
			processorCount++
		case "flags":
			if strings.Contains(value, "sse4") {
				microarch = "SSE4"
			}
		}
	}
	if processorCount == 0 {
		processorCount = 1
	}
	if vendor == "AuthenticAMD" || strings.Contains(strings.ToLower(vendor), "amd") {
		info.Vendor = "AuthenticAMD"
	} else if vendor == "GenuineIntel" || strings.Contains(strings.ToLower(vendor), "intel") {
		info.Vendor = "GenuineIntel"
	} else {
		info.Vendor = vendor
	}
	if model != "" {
		info.Model = model
	}
	if microarch != "" {
		info.Microarchitecture = microarch
	} else {
		info.Microarchitecture = "Generic"
	}
	if baseMHz != "" {
		info.BaseFrequencyMHz = baseMHz + " MHz"
	}
	if maxMHz != "" {
		info.MaxFrequencyMHz = maxMHz + " MHz"
	} else if baseMHz != "" {
		info.MaxFrequencyMHz = baseMHz + " MHz"
	}
	info.TotalCores = max(1, processorCount)
	info.TotalThreads = max(1, processorCount)
	return info
}

func readCPUStats(previous []cpuSample) ([]float64, []cpuSample, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	current := make([]cpuSample, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !isCPUCoreStatLine(line) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		values := make([]uint64, 0, len(parts)-1)
		for _, part := range parts[1:] {
			v, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				continue
			}
			values = append(values, v)
		}
		if len(values) < 8 {
			continue
		}
		current = append(current, cpuSample{user: values[0], nice: values[1], system: values[2], idle: values[3], iowait: values[4], irq: values[5], softirq: values[6], steal: values[7]})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	loads := make([]float64, 0, len(current))
	for i := range current {
		load := 0.0
		if i < len(previous) {
			prev := previous[i]
			curr := current[i]
			totalDelta := (curr.user + curr.nice + curr.system + curr.idle + curr.iowait + curr.irq + curr.softirq + curr.steal) - (prev.user + prev.nice + prev.system + prev.idle + prev.iowait + prev.irq + prev.softirq + prev.steal)
			idleDelta := (curr.idle + curr.iowait) - (prev.idle + prev.iowait)
			if totalDelta > 0 {
				load = 100.0 * float64(totalDelta-idleDelta) / float64(totalDelta)
			}
		}
		loads = append(loads, load)
	}
	return loads, current, nil
}

func readTopProcess() string {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "telemetry"
	}
	return strings.TrimSpace(string(data))
}

func readMemoryUsage() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, available uint64
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "MemTotal:":
			value, err := strconv.ParseUint(parts[1], 10, 64)
			if err == nil {
				total = value
			}
		case "MemAvailable:":
			value, err := strconv.ParseUint(parts[1], 10, 64)
			if err == nil {
				available = value
			}
		}
	}
	if total == 0 {
		return 0
	}
	return 100.0 * float64(total-available) / float64(total)
}

func renderMemoryBar(width int, percent int) string {
	if width < 1 {
		width = 1
	}
	filled := int(float64(percent) / 100.0 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#74c7ec")).Render(bar)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
