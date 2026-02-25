use std::{
    io,
    process::Command,
    time::{Duration, Instant},
};

use crossterm::{
    event::{self, Event, KeyCode, KeyEventKind},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};

use ratatui::{
    backend::CrosstermBackend,
    layout::{Constraint, Direction, Layout},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Tabs},
    Terminal,
};

#[derive(Clone)]
struct Task {
    name: &'static str,
    command: Vec<&'static str>,
    selected: bool,
}

struct App {
    tasks: Vec<Task>,
    state: usize,
    list_state: ListState,
    logs: Vec<String>,
    running: bool,
}

impl App {
    fn new() -> Self {
        let mut list_state = ListState::default();
        list_state.select(Some(0));

        Self {
            tasks: vec![
                Task { name: "Limpiar TEMP", command: vec!["cmd", "/C", "del /q/f/s %TEMP%\\*"], selected: true },
                Task { name: "Limpiar Prefetch", command: vec!["cmd", "/C", "del /q/f/s C:\\Windows\\Prefetch\\*"], selected: true },
                Task { name: "Flush DNS", command: vec!["ipconfig", "/flushdns"], selected: true },
                Task { name: "Reset Winsock", command: vec!["netsh", "winsock", "reset"], selected: true },
                Task { name: "Reset IP", command: vec!["netsh", "int", "ip", "reset"], selected: true },
                Task { name: "DISM Cleanup", command: vec!["dism", "/online", "/cleanup-image", "/startcomponentcleanup"], selected: true },
                Task { name: "SFC Verify", command: vec!["sfc", "/verifyonly"], selected: true },
                Task { name: "Desactivar Telemetría", command: vec!["sc", "stop", "DiagTrack"], selected: false },
                Task { name: "Limpiar Windows Update", command: vec!["cmd", "/C", "rd /s /q C:\\Windows\\SoftwareDistribution\\Download"], selected: true },
                Task { name: "Optimizar HDD", command: vec!["defrag", "C:", "/U", "/X"], selected: false },
            ],
            state: 0,
            list_state,
            logs: vec!["Sistema listo.".into()],
            running: false,
        }
    }

    fn next(&mut self) {
        let i = match self.list_state.selected() {
            Some(i) => {
                if i >= self.tasks.len() - 1 { 0 } else { i + 1 }
            }
            None => 0,
        };
        self.list_state.select(Some(i));
    }

    fn previous(&mut self) {
        let i = match self.list_state.selected() {
            Some(i) => {
                if i == 0 { self.tasks.len() - 1 } else { i - 1 }
            }
            None => 0,
        };
        self.list_state.select(Some(i));
    }

    fn toggle(&mut self) {
        if let Some(i) = self.list_state.selected() {
            self.tasks[i].selected = !self.tasks[i].selected;
        }
    }

    fn run_tasks(&mut self) {
        self.running = true;
        self.logs.clear();
        self.logs.push("Iniciando optimización...\n".into());

        for task in self.tasks.iter() {
            if task.selected {
                self.logs.push(format!("Ejecutando: {}", task.name));
                let _ = Command::new(task.command[0])
                    .args(&task.command[1..])
                    .output();
            }
        }

        self.logs.push("\nOptimización completada.".into());
        self.running = false;
    }
}

fn main() -> io::Result<()> {
    enable_raw_mode()?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen)?;

    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let res = run_app(&mut terminal);

    disable_raw_mode()?;
    execute!(terminal.backend_mut(), LeaveAlternateScreen)?;
    terminal.show_cursor()?;

    res
}

fn run_app(terminal: &mut Terminal<CrosstermBackend<std::io::Stdout>>) -> io::Result<()> {
    let mut app = App::new();
    let tick_rate = Duration::from_millis(200);
    let mut last_tick = Instant::now();

    loop {
        terminal.draw(|f| ui(f, &mut app))?;

        let timeout = tick_rate
            .checked_sub(last_tick.elapsed())
            .unwrap_or_else(|| Duration::from_secs(0));

        if event::poll(timeout)? {
            if let Event::Key(key) = event::read()? {
                if key.kind == KeyEventKind::Press {
                    match key.code {
                        KeyCode::Char('q') => return Ok(()),
                        KeyCode::Down => app.next(),
                        KeyCode::Up => app.previous(),
                        KeyCode::Char(' ') => app.toggle(),
                        KeyCode::Enter => app.run_tasks(),
                        KeyCode::Esc => break Ok(()), // ✅ CORREGIDO
                        _ => {}
                    }
                }
            }
        }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }
    }
}

fn ui(f: &mut ratatui::Frame, app: &mut App) {
    let size = f.size();

    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .margin(2)
        .constraints([
            Constraint::Length(3),
            Constraint::Min(5),
            Constraint::Length(7),
        ])
        .split(size);

    let titles: Vec<Line> = ["Selección", "Ejecución", "Logs"]
        .iter()
        .map(|t| Line::from(*t))
        .collect();

    let tabs = Tabs::new(titles)
        .select(0)
        .block(Block::default().borders(Borders::ALL).title("Windows Turbo Optimizer"))
        .highlight_style(Style::default().fg(Color::Cyan).add_modifier(Modifier::BOLD));

    f.render_widget(tabs, chunks[0]);

    let items: Vec<ListItem> = app
        .tasks
        .iter()
        .map(|t| {
            let status = if t.selected { "[x]" } else { "[ ]" };
            ListItem::new(Line::from(vec![
                Span::styled(status, Style::default().fg(Color::Green)),
                Span::raw(" "),
                Span::raw(t.name),
            ]))
        })
        .collect();

    let list = List::new(items)
        .block(Block::default().borders(Borders::ALL).title("Optimización"))
        .highlight_style(Style::default().bg(Color::Blue));

    f.render_stateful_widget(list, chunks[1], &mut app.list_state);

    let log_text = app.logs.join("\n");

    let logs = Paragraph::new(log_text)
        .block(Block::default().borders(Borders::ALL).title("Logs"))
        .style(Style::default().fg(Color::White));

    f.render_widget(logs, chunks[2]);
}