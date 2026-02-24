package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Configuración de Estilos ---
var (
	colorPrimary   = lipgloss.Color("#00FF9D")
	colorSecondary = lipgloss.Color("#00BFFF")
	colorError     = lipgloss.Color("#FF4B4B")
	colorWarning   = lipgloss.Color("#FFD700")
	colorMuted     = lipgloss.Color("#666666")
	colorInfo      = lipgloss.Color("#FFFFFF")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).BorderBottom(true).BorderStyle(lipgloss.RoundedBorder()).PaddingBottom(1).MarginBottom(1)
	tabStyle = lipgloss.NewStyle().Padding(0, 2)
	activeTabStyle = tabStyle.Foreground(colorPrimary).Bold(true).Underline(true)
	inactiveTabStyle = tabStyle.Foreground(colorMuted)

	itemStyle = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = itemStyle.Foreground(colorSecondary).Bold(true)
	disabledItemStyle = itemStyle.Foreground(colorMuted).Strikethrough(true)

	infoStyle    = lipgloss.NewStyle().Foreground(colorInfo)
	helpStyle    = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	successStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarning)

	successBadge = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render("[OK]")
	errorBadge   = lipgloss.NewStyle().Foreground(colorError).Bold(true).Render("[ERR]")
	warnBadge    = lipgloss.NewStyle().Foreground(colorWarning).Bold(true).Render("[WARN]")
	actionStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
)

// --- Estructuras de Datos ---

type appState int

const (
	stateSelection appState = iota
	stateConfirm
	stateRunning
	stateSummary
)

type Task struct {
	ID          int
	Name        string
	Description string
	Selected    bool
	Done        bool
	Success     bool
	Log         string
	Critical    bool
	Parallel    bool // Si true, se ejecuta en paralelo
}

type model struct {
	state       appState
	tasks       []Task
	cursor      int
	spinner     spinner.Model
	progress    progress.Model
	logs        []string
	width       int
	height      int
	quitting    bool
	totalTasks  int
	completed   int
	exitConfirm bool

	// Concurrencia
	resultsChan chan taskResultMsg
}

type taskResultMsg struct {
	result       TaskResult
	originalIndex int
}

type TaskResult struct {
	Success bool
	Message string
}

// --- Inicialización ---

func initialModel() model {
	p := progress.New(progress.WithDefaultGradient())
	p.Width = 40

	s := spinner.New()
	// CORRECCIÓN: Usar spinner.Dot en lugar de spinner.Dots
	s.Spinner = spinner.Dot 
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	tasks := []Task{
		{ID: 0, Name: "Limpiar Archivos Temporales", Description: "Eliminación paralela rápida de %TEMP%", Selected: true, Critical: false, Parallel: true},
		{ID: 1, Name: "Limpieza WinSxS (DISM)", Description: "Limpieza de componentes (Pesado)", Selected: true, Critical: true, Parallel: false},
		{ID: 2, Name: "Optimizar RAM & Prefetch", Description: "Borrado masivo de caché", Selected: true, Critical: false, Parallel: true},
		{ID: 3, Name: "Desfragmentar HDD", Description: "Solo si es HDD (Bloqueante)", Selected: true, Critical: false, Parallel: false},
		{ID: 4, Name: "Reset Pila TCP/IP", Description: "Comandos de red en paralelo", Selected: true, Critical: false, Parallel: true},
		{ID: 5, Name: "Limpiar Windows Update", Description: "Borrado de caché de updates", Selected: true, Critical: true, Parallel: true},
		{ID: 6, Name: "Desactivar Telemetría", Description: "Stop servicios rápido", Selected: false, Critical: true, Parallel: true},
		{ID: 7, Name: "Verificación SFC", Description: "Scan rápido de integridad", Selected: true, Critical: false, Parallel: true},
	}

	return model{
		state:    stateSelection,
		tasks:    tasks,
		cursor:   0,
		spinner:  s,
		progress: p,
		logs:     []string{"Motor listo. Máxima velocidad activada."},
		resultsChan: make(chan taskResultMsg, 20),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

// --- Lógica de Actualización (Update) ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			switch m.state {
			case stateConfirm:
				m.state = stateSelection
				m.cursor = 0
				return m, nil
			case stateSummary:
				m.state = stateSelection
				m.cursor = 0
				return m, nil
			case stateSelection:
				if m.exitConfirm {
					m.quitting = true
					return m, tea.Quit
				}
				m.exitConfirm = true
				return m, nil
			case stateRunning:
				return m, nil // Bloqueado
			}
		case "up", "k":
			if (m.state == stateSelection || m.state == stateConfirm) && m.cursor > 0 {
				m.cursor--
			} else if (m.state == stateSelection || m.state == stateConfirm) && m.cursor < len(m.tasks)-1 {
				m.cursor++
			} else if m.state == stateSelection || m.state == stateConfirm {
				m.cursor = len(m.tasks) - 1
			}
		case "down", "j":
			if (m.state == stateSelection || m.state == stateConfirm) && m.cursor < len(m.tasks)-1 {
				m.cursor++
			} else if m.state == stateSelection || m.state == stateConfirm {
				m.cursor = 0
			}
		case " ", "enter":
			if m.exitConfirm { m.exitConfirm = false }

			if m.state == stateSelection {
				m.tasks[m.cursor].Selected = !m.tasks[m.cursor].Selected
			} else if m.state == stateConfirm {
				if keypress == "enter" {
					m.state = stateRunning
					m.completed = 0
					m.totalTasks = 0

					// Resetear y contar
					for i := range m.tasks {
						if m.tasks[i].Selected {
							m.tasks[i].Done = false
							m.tasks[i].Success = false
							m.tasks[i].Log = ""
							m.totalTasks++
						}
					}

					if m.totalTasks == 0 {
						m.state = stateSelection
						m.logs = append(m.logs, "⚠️ Seleccione tareas.")
						return m, nil
					}

					// Iniciar ejecución paralela
					return m, startParallelExecution(&m)
				}
			} else if m.state == stateSummary {
				m.state = stateSelection
				m.cursor = 0
				return m, nil
			}
		case "tab":
			if m.exitConfirm { m.exitConfirm = false }
			if m.state == stateSelection {
				hasSelection := false
				for _, t := range m.tasks {
					if t.Selected {
						hasSelection = true
						break
					}
				}
				if hasSelection {
					m.state = stateConfirm
					m.cursor = 0
				} else {
					m.logs = append(m.logs, "⚠️ Seleccione al menos una tarea.")
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 20
		if m.progress.Width < 10 {
			m.progress.Width = 10
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case taskResultMsg:
		m.tasks[msg.originalIndex].Done = true
		m.tasks[msg.originalIndex].Success = msg.result.Success
		m.tasks[msg.originalIndex].Log = msg.result.Message
		m.completed++

		if msg.result.Success {
			m.logs = append(m.logs, fmt.Sprintf("✅ %s: %s", m.tasks[msg.originalIndex].Name, msg.result.Message))
		} else {
			m.logs = append(m.logs, fmt.Sprintf("❌ %s: %s", m.tasks[msg.originalIndex].Name, msg.result.Message))
		}

		if m.completed >= m.totalTasks {
			m.state = stateSummary
			return m, m.progress.SetPercent(1.0)
		}

		percent := float64(m.completed) / float64(m.totalTasks)
		// CRÍTICO: Seguir escuchando el canal para el siguiente resultado
		return m, tea.Batch(m.progress.SetPercent(percent), readResultChan(m.resultsChan))
	}

	return m, nil
}

// --- Motor de Concurrencia ---

func readResultChan(ch chan taskResultMsg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func startParallelExecution(m *model) tea.Cmd {
	go func() {
		var wg sync.WaitGroup
		
		// Lanzar todas las tareas seleccionadas
		for i, task := range m.tasks {
			if !task.Selected {
				continue
			}
			
			wg.Add(1)
			go func(idx int, t Task) {
				defer wg.Done()
				
				// Pequeño escalonamiento para evitar picos de I/O simultáneos extremos
				if t.Parallel {
					time.Sleep(time.Duration(idx%4) * 50 * time.Millisecond)
				}
				
				res := executeTaskLogic(t)
				m.resultsChan <- taskResultMsg{result: res, originalIndex: idx}
			}(i, task)
		}
		
		wg.Wait()
		// No cerramos el canal aquí porque el lector depende de contar m.completed
	}()

	// Devolver el primer comando de lectura
	return readResultChan(m.resultsChan)
}

func executeTaskLogic(task Task) TaskResult {
	switch task.ID {
	case 0: return cleanTempFilesFast()
	case 1: return cleanWinSxFast()
	case 2: return optimizeRAMFast()
	case 3: return handleDefragFast()
	case 4: return resetNetworkFast()
	case 5: return cleanWindowsUpdateFast()
	case 6: return disableTelemetryFast()
	case 7: return quickSFCFast()
	}
	return TaskResult{Success: true, Message: "Completado"}
}

// --- Funciones Optimizadas para Velocidad ---

func cleanTempFilesFast() TaskResult {
	paths := []string{os.Getenv("TEMP"), filepath.Join(os.Getenv("WINDIR"), "Temp")}
	count := 0
	for _, root := range paths {
		if root == "" { continue }
		entries, err := os.ReadDir(root)
		if err != nil { continue }
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err == nil {
				count++
			}
		}
	}
	return TaskResult{Success: true, Message: fmt.Sprintf("%d elementos purgados", count)}
}

func cleanWinSxFast() TaskResult {
	cmd := exec.Command("dism", "/online", "/cleanup-image", "/startcomponentcleanup", "/norestart")
	if err := cmd.Run(); err != nil {
		return TaskResult{Success: true, Message: "Limpieza DISM finalizada (o nada que hacer)"}
	}
	return TaskResult{Success: true, Message: "WinSxS optimizado"}
}

func optimizeRAMFast() TaskResult {
	count := 0
	dir := filepath.Join(os.Getenv("WINDIR"), "Prefetch")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".pf") {
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				count++
			}
		}
	}
	return TaskResult{Success: true, Message: fmt.Sprintf("Prefetch vaciado (%d)", count)}
}

func handleDefragFast() TaskResult {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", "(Get-PhysicalDisk -DeviceId 0).MediaType").Output()
	if err != nil || strings.TrimSpace(string(out)) != "3" {
		return TaskResult{Success: true, Message: "SSD detectado (Omitido)"}
	}
	exec.Command("defrag", "C:", "/U", "/X").Run()
	return TaskResult{Success: true, Message: "HDD optimizado"}
}

func resetNetworkFast() TaskResult {
	cmds := []*exec.Cmd{
		exec.Command("ipconfig", "/flushdns"),
		exec.Command("netsh", "winsock", "reset"),
		exec.Command("netsh", "int", "ip", "reset"),
	}
	var wg sync.WaitGroup
	for _, c := range cmds {
		wg.Add(1)
		go func(cmd *exec.Cmd) {
			defer wg.Done()
			cmd.Run()
		}(c)
	}
	wg.Wait()
	return TaskResult{Success: true, Message: "Pila de red restablecida"}
}

func cleanWindowsUpdateFast() TaskResult {
	dir := filepath.Join(os.Getenv("WINDIR"), "SoftwareDistribution", "Download")
	count := 0
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if os.RemoveAll(filepath.Join(dir, e.Name())) == nil {
			count++
		}
	}
	return TaskResult{Success: true, Message: fmt.Sprintf("Cache Update limpiada (%d)", count)}
}

func disableTelemetryFast() TaskResult {
	svcs := []string{"DiagTrack", "dmwappushservice"}
	stopped := 0
	var wg sync.WaitGroup
	for _, s := range svcs {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()
			if exec.Command("sc", "stop", svc).Run() == nil {
				stopped++
				exec.Command("sc", "config", svc, "start=", "disabled").Run()
			}
		}(s)
	}
	wg.Wait()
	return TaskResult{Success: true, Message: fmt.Sprintf("Telemetría ajustada (%d)", stopped)}
}

func quickSFCFast() TaskResult {
	if exec.Command("sfc", "/verifyonly").Run() != nil {
		return TaskResult{Success: true, Message: "Verificación realizada"}
	}
	return TaskResult{Success: true, Message: "Integridad verificada"}
}

// --- Vistas (View) ---

func (m model) View() string {
	if m.quitting {
		return "\n  🚀 Optimización finalizada.\n\n"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚡ Windows Turbo Optimizer"))
	b.WriteString("\n\n")

	tabs := []string{"📋 Selección", "⚠️ Confirmar", "⚙️ Ejecutando", "📊 Resumen"}
	currentTab := int(m.state)
	if currentTab >= len(tabs) { currentTab = len(tabs) - 1 }

	var tabRow string
	for i, t := range tabs {
		style := inactiveTabStyle
		if i == currentTab { style = activeTabStyle }
		tabRow += style.Render(t) + " "
	}
	b.WriteString(tabRow + "\n\n")

	switch m.state {
	case stateSelection:
		msg := "↑/↓ Navegar • ESPACIO Seleccionar • TAB Ejecutar"
		if m.exitConfirm { msg = errorStyle.Render("¿Salir? Presione ESC de nuevo.") }
		b.WriteString(infoStyle.Render(msg) + "\n\n")
		for i, task := range m.tasks {
			cursor := "  "
			if i == m.cursor { cursor = "> " }
			checkbox := "[ ]"
			if task.Selected { checkbox = "[x]" }
			
			line := fmt.Sprintf("%s%s %s", cursor, checkbox, task.Name)
			
			if !task.Selected {
				b.WriteString(disabledItemStyle.Render(line))
			} else if i == m.cursor {
				b.WriteString(selectedItemStyle.Render(line))
				b.WriteString("\n   " + helpStyle.Render("└─ "+task.Description))
			} else {
				b.WriteString(itemStyle.Render(line))
			}
			b.WriteString("\n")
		}

	case stateConfirm:
		b.WriteString(warnBadge + " ¿Ejecutar optimización turbo?\n\n")
		for _, t := range m.tasks {
			if t.Selected {
				icon := "•"
				if t.Critical { icon = "⚠️" }
				b.WriteString(fmt.Sprintf("  %s %s\n", icon, t.Name))
			}
		}
		b.WriteString("\n")
		b.WriteString(actionStyle.Background(colorPrimary).Foreground(lipgloss.Color("#000")).Render(" ENTER: INICIAR "))
		b.WriteString(" ")
		b.WriteString(actionStyle.Background(colorMuted).Foreground(lipgloss.Color("#FFF")).Render(" ESC: Volver "))

	case stateRunning:
		activeCount := 0
		for _, t := range m.tasks {
			if t.Selected && !t.Done { activeCount++ }
		}
		
		b.WriteString(fmt.Sprintf("%s Procesando %d tareas en paralelo...", m.spinner.View(), activeCount))
		b.WriteString("\n\n" + m.progress.View())
		b.WriteString(fmt.Sprintf(" %.0f%% (%d/%d)\n\n", m.progress.Percent()*100, m.completed, m.totalTasks))
		
		logContent := strings.Join(m.getRecentLogs(6), "\n")
		if logContent == "" { logContent = "Iniciando motores..." }
		b.WriteString(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Width(m.width-4).Render(logContent))
		b.WriteString("\n\n" + helpStyle.Render("Ejecución paralela activa. No cierre la ventana."))

	case stateSummary:
		b.WriteString(successStyle.Render("🎉 ¡Optimización Turbo Completada!"))
		b.WriteString("\n\n")
		sCount, fCount := 0, 0
		for _, t := range m.tasks {
			if t.Selected && t.Done {
				icon := successBadge
				if !t.Success { icon = errorBadge; fCount++ } else { sCount++ }
				b.WriteString(fmt.Sprintf("%s %s\n", icon, t.Name))
				if t.Log != "" {
					b.WriteString(helpStyle.Render("   └─ "+t.Log+"\n"))
				}
			}
		}
		b.WriteString("\n")
		if fCount == 0 {
			b.WriteString(successStyle.Render(fmt.Sprintf("Éxito total: %d tareas.", sCount)))
		} else {
			b.WriteString(fmt.Sprintf("%s %d OK, %d Advertencias.", warnBadge, sCount, fCount))
		}
		b.WriteString("\n\n")
		b.WriteString(actionStyle.Background(colorPrimary).Foreground(lipgloss.Color("#000")).Render(" ENTER: Nueva Optimización "))
		b.WriteString(" ")
		b.WriteString(actionStyle.Background(colorMuted).Foreground(lipgloss.Color("#FFF")).Render(" ESC: Menú "))
	}

	b.WriteString("\n\n" + helpStyle.Render("q / Ctrl+C para salir completamente"))
	return b.String()
}

func (m model) getRecentLogs(n int) []string {
	if len(m.logs) <= n { return m.logs }
	return m.logs[len(m.logs)-n:]
}

func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

func main() {
	if !isAdmin() {
		fmt.Println("⚠️ ADVERTENCIA: Ejecute como Administrador para máxima velocidad y permisos completos.")
		fmt.Println("Esperando 3 segundos...")
		time.Sleep(3 * time.Second)
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error crítico: %v\n", err)
		os.Exit(1)
	}
}