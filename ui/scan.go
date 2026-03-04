package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// FileStatus represents the scan status of a file
type FileStatus int

const (
	StatusPending FileStatus = iota
	StatusScanning
	StatusComplete
	StatusMissing
	StatusError
)

// FileInfo holds information about a file being scanned
type FileInfo struct {
	Path        string
	Title       string
	Album       string
	Artist      string
	Status      FileStatus
	MissingTags []string
	Error       string
}

// ScanModel is the bubbletea model for the scan UI
type ScanModel struct {
	// Content info
	Folder string

	// Files
	Files        []FileInfo
	TotalFiles   int
	CurrentIndex int

	// UI components
	spinner  spinner.Model
	progress progress.Model

	// State
	Done      bool
	Cancelled bool
	StartTime time.Time

	// Stats
	CompleteCount int
	MissingCount  int
	ErrorCount    int

	// Thread safety
	mu sync.Mutex
}

// Messages
type TickMsg time.Time

type FileFoundMsg struct {
	Total int
}

type FileStartMsg struct {
	Index int
}

type FileCompleteMsg struct {
	Index       int
	MissingTags []string
	Album       string
	Artist      string
	Title       string
}

type FileErrorMsg struct {
	Index int
	Error string
}

type ScanCompleteMsg struct{}

// NewScanModel creates a new scan UI model
func NewScanModel(folder string) *ScanModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return &ScanModel{
		Folder:    folder,
		spinner:   s,
		progress:  p,
		StartTime: time.Now(),
		Files:     make([]FileInfo, 0),
	}
}

// Init initializes the model
func (m *ScanModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Update handles messages
func (m *ScanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.Cancelled = true
			return m, tea.Quit
		}

	case TickMsg:
		return m, tickCmd()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case FileFoundMsg:
		m.mu.Lock()
		m.TotalFiles = msg.Total
		m.Files = make([]FileInfo, msg.Total)
		m.mu.Unlock()

	case FileStartMsg:
		m.mu.Lock()
		if msg.Index < len(m.Files) {
			m.Files[msg.Index].Status = StatusScanning
			m.CurrentIndex = msg.Index
		}
		m.mu.Unlock()

	case FileCompleteMsg:
		m.mu.Lock()
		if msg.Index < len(m.Files) {
			m.Files[msg.Index].Title = msg.Title
			m.Files[msg.Index].Album = msg.Album
			m.Files[msg.Index].Artist = msg.Artist
			m.Files[msg.Index].MissingTags = msg.MissingTags
			if len(msg.MissingTags) > 0 {
				m.Files[msg.Index].Status = StatusMissing
				m.MissingCount++
			} else {
				m.Files[msg.Index].Status = StatusComplete
				m.CompleteCount++
			}
		}
		m.mu.Unlock()

	case FileErrorMsg:
		m.mu.Lock()
		if msg.Index < len(m.Files) {
			m.Files[msg.Index].Status = StatusError
			m.Files[msg.Index].Error = msg.Error
			m.ErrorCount++
		}
		m.mu.Unlock()

	case ScanCompleteMsg:
		m.Done = true
		return m, tea.Quit

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	return m, nil
}

// View renders the UI
func (m *ScanModel) View() string {
	if m.Cancelled {
		return MutedStyle.Render("Scan cancelled.\n")
	}

	var b strings.Builder

	// Header
	header := fmt.Sprintf("%s %s", IconFolder, TitleStyle.Render("Checking music files"))
	b.WriteString(header + "\n")
	b.WriteString(MutedStyle.Render(m.Folder) + "\n\n")

	// Show files being processed (limit visible to last N files)
	m.mu.Lock()
	files := m.Files
	totalFiles := m.TotalFiles
	completeCount := m.CompleteCount
	missingCount := m.MissingCount
	errorCount := m.ErrorCount
	m.mu.Unlock()

	processedCount := completeCount + missingCount + errorCount

	// Show last 10 processed files + currently processing
	maxVisible := 10
	startIdx := 0
	if processedCount > maxVisible {
		startIdx = processedCount - maxVisible
	}

	for i := startIdx; i < len(files) && i < processedCount+5; i++ {
		file := files[i]
		if file.Status == StatusPending {
			continue
		}
		m.renderFile(&b, i, file)
	}

	// Summary
	b.WriteString("\n")
	elapsed := time.Since(m.StartTime).Round(time.Second)

	if m.Done {
		var summary string
		if missingCount > 0 {
			summary = fmt.Sprintf("%s Scan complete: %d/%d files have all tags, %d missing tags",
				WarningStyle.Render(IconWarning),
				completeCount,
				totalFiles,
				missingCount,
			)
		} else {
			summary = fmt.Sprintf("%s All %d files have complete tags!",
				SuccessStyle.Render(IconCheck),
				totalFiles,
			)
		}
		if errorCount > 0 {
			summary += fmt.Sprintf(" (%d errors)", errorCount)
		}
		summary += fmt.Sprintf(" in %s", elapsed)
		b.WriteString(BoxStyle.Render(summary))
		b.WriteString("\n")
	} else {
		// Overall progress
		var overallProgress float64
		if totalFiles > 0 {
			overallProgress = float64(processedCount) / float64(totalFiles)
		}
		progressBar := m.progress.ViewAs(overallProgress)

		summary := fmt.Sprintf("Progress: %s %d/%d files • Elapsed: %s",
			progressBar,
			processedCount,
			totalFiles,
			elapsed,
		)
		b.WriteString(MutedStyle.Render(summary))
		b.WriteString("\n")
	}

	// Help
	if !m.Done {
		b.WriteString(HelpStyle.Render("Press q to quit"))
	}

	return b.String()
}

func (m *ScanModel) renderFile(b *strings.Builder, index int, file FileInfo) {
	// Track number
	num := TrackNumberStyle.Render(fmt.Sprintf("[%d]", index+1))

	// Status icon and styling
	var status, title string
	switch file.Status {
	case StatusPending:
		status = MutedStyle.Render(IconDot)
		title = MutedStyle.Render(file.Title)
	case StatusScanning:
		status = m.spinner.View()
		title = TrackTitleStyle.Render(file.Title)
		if file.Title == "" {
			title = MutedStyle.Render("scanning...")
		}
	case StatusComplete:
		status = SuccessStyle.Render(IconCheck)
		title = TrackTitleStyle.Render(file.Title)
	case StatusMissing:
		status = WarningStyle.Render(IconWarning)
		title = TrackTitleStyle.Render(file.Title)
	case StatusError:
		status = ErrorStyle.Render(IconCross)
		title = ErrorStyle.Render(file.Title)
	}

	// First line: status, number, title
	line := fmt.Sprintf("%s %s %s", status, num, title)

	// If missing tags, show which ones
	if file.Status == StatusMissing && len(file.MissingTags) > 0 {
		line += " " + MutedStyle.Render("→ missing: "+strings.Join(file.MissingTags, ", "))
	}

	// If error, show error message
	if file.Status == StatusError && file.Error != "" {
		line += " " + ErrorStyle.Render(fmt.Sprintf("(%s)", file.Error))
	}

	b.WriteString(line + "\n")
}

// GetStats returns scan statistics
func (m *ScanModel) GetStats() (total, complete, missing, errors int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.TotalFiles, m.CompleteCount, m.MissingCount, m.ErrorCount
}

// GetMissingFiles returns files that have missing tags
func (m *ScanModel) GetMissingFiles() []FileInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	var missing []FileInfo
	for _, f := range m.Files {
		if f.Status == StatusMissing {
			missing = append(missing, f)
		}
	}
	return missing
}

// IsCancelled returns whether the scan was cancelled
func (m *ScanModel) IsCancelled() bool {
	return m.Cancelled
}
