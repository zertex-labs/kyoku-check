package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// Colors
var (
	colorPrimary = lipgloss.Color("#7C3AED") // Purple
	colorSuccess = lipgloss.Color("#10B981") // Green
	colorMuted   = lipgloss.Color("#6B7280") // Gray
)

// Styles
var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	mutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
)

// Icons
const (
	iconCheck = "✓"
)

// supportedExtensions are the audio file types we scan
var supportedExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".m4a":  true,
	".ogg":  true,
	".opus": true,
}

// TrackInfo holds metadata extracted from a music file
type TrackInfo struct {
	Path   string
	Artist string
	Album  string
	Title  string
	// All tags from the file (uppercase keys)
	Tags map[string]string
}

// MissingTags returns a list of missing tag names based on config
func (t *TrackInfo) MissingTags(requiredTags []string) []string {
	var missing []string
	for _, tag := range requiredTags {
		upperTag := strings.ToUpper(tag)
		if t.Tags[upperTag] == "" {
			missing = append(missing, tag)
		}
	}
	return missing
}

// HasAllTags returns true if all required tags are present
func (t *TrackInfo) HasAllTags(requiredTags []string) bool {
	return len(t.MissingTags(requiredTags)) == 0
}

// Stats holds scan statistics
type Stats struct {
	TotalFiles    int
	CompleteFiles int
	MissingFiles  int
	Errors        int
}

// ScanModel is the bubbletea model for the scanning UI
type ScanModel struct {
	folder      string
	spinner     spinner.Model
	currentFile string
	stats       Stats
	done        bool
	err         error
	tracks      []TrackInfo // Tracks missing tags
	mu          sync.Mutex
	startTime   time.Time
}

// Messages
type scanCompleteMsg struct {
	tracks []TrackInfo
	stats  Stats
	err    error
}

func newScanModel(folder string) *ScanModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	return &ScanModel{
		folder:    folder,
		spinner:   s,
		startTime: time.Now(),
	}
}

func (m *ScanModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startScan())
}

func (m *ScanModel) startScan() tea.Cmd {
	return func() tea.Msg {
		var tracks []TrackInfo
		var stats Stats

		err := filepath.WalkDir(m.folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if !supportedExtensions[ext] {
				return nil
			}

			stats.TotalFiles++

			// Update current file being scanned
			m.mu.Lock()
			m.currentFile = path
			m.stats = stats
			m.mu.Unlock()

			track, err := extractMetadata(path)
			if err != nil {
				stats.Errors++
				return nil
			}

			if track.HasAllTags(appConfig.Tags) {
				stats.CompleteFiles++
			} else {
				stats.MissingFiles++
				tracks = append(tracks, track)
			}

			return nil
		})

		return scanCompleteMsg{tracks: tracks, stats: stats, err: err}
	}
}

func (m *ScanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case scanCompleteMsg:
		m.done = true
		m.tracks = msg.tracks
		m.stats = msg.stats
		m.err = msg.err
		return m, tea.Quit
	}

	return m, nil
}

func (m *ScanModel) View() string {
	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("Scanning for music files") + "\n")
	b.WriteString(mutedStyle.Render(m.folder) + "\n\n")

	if m.done {
		elapsed := time.Since(m.startTime).Round(time.Millisecond)
		b.WriteString(successStyle.Render(iconCheck) + " Scan complete in " + elapsed.String() + "\n\n")
	} else {
		// Show spinner and current file
		m.mu.Lock()
		currentFile := m.currentFile
		stats := m.stats
		m.mu.Unlock()

		b.WriteString(m.spinner.View() + " Scanning...\n")
		if currentFile != "" {
			// Show just the filename, not full path
			displayPath := currentFile
			if rel, err := filepath.Rel(m.folder, currentFile); err == nil {
				displayPath = rel
			}
			// Truncate if too long
			if len(displayPath) > 60 {
				displayPath = "..." + displayPath[len(displayPath)-57:]
			}
			b.WriteString(mutedStyle.Render("  "+displayPath) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Files scanned: %d", stats.TotalFiles)) + "\n")
	}

	return b.String()
}

func runCheck(cmd *cobra.Command, args []string) error {
	folder := args[0]

	// Check if ffprobe is available
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe not found in PATH; please install ffmpeg")
	}

	// Check if folder exists
	info, err := os.Stat(folder)
	if err != nil {
		return fmt.Errorf("cannot access folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", folder)
	}

	// Resolve to absolute path
	absFolder, err := filepath.Abs(folder)
	if err != nil {
		absFolder = folder
	}

	var tracks []TrackInfo
	var stats Stats

	// Try to run with TUI, fall back to simple output if TTY not available
	model := newScanModel(absFolder)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		// Fallback to non-interactive mode
		tracks, stats, err = runScanSimple(absFolder)
		if err != nil {
			return err
		}
	} else {
		m := finalModel.(*ScanModel)
		if m.err != nil {
			return fmt.Errorf("scan error: %w", m.err)
		}
		tracks = m.tracks
		stats = m.stats
	}

	// Sort tracks based on format
	if appConfig.Format == "artist" {
		// Sort by artist -> album -> title
		sort.Slice(tracks, func(i, j int) bool {
			if tracks[i].Artist != tracks[j].Artist {
				return tracks[i].Artist < tracks[j].Artist
			}
			if tracks[i].Album != tracks[j].Album {
				return tracks[i].Album < tracks[j].Album
			}
			return tracks[i].Title < tracks[j].Title
		})
	} else {
		// Sort by album -> title (default)
		sort.Slice(tracks, func(i, j int) bool {
			if tracks[i].Album != tracks[j].Album {
				return tracks[i].Album < tracks[j].Album
			}
			return tracks[i].Title < tracks[j].Title
		})
	}

	// Write output file if there are missing files
	if len(tracks) > 0 {
		if err := writeReport(tracks, outputFile, appConfig); err != nil {
			return fmt.Errorf("failed to write report: %w", err)
		}
	}

	// Print statistics
	fmt.Println("\nStatistics:")
	fmt.Printf("  Total files scanned: %d\n", stats.TotalFiles)
	fmt.Printf("  Files with all tags: %d\n", stats.CompleteFiles)
	fmt.Printf("  Files missing tags:  %d\n", stats.MissingFiles)
	if stats.Errors > 0 {
		fmt.Printf("  Errors:              %d\n", stats.Errors)
	}

	if len(tracks) > 0 {
		absPath, _ := filepath.Abs(outputFile)
		fmt.Printf("\nReport written to: %s\n", absPath)
	} else {
		fmt.Println("\nAll files have complete tags!")
	}

	return nil
}

// runScanSimple runs the scan without TUI (fallback for non-TTY environments)
func runScanSimple(folder string) ([]TrackInfo, Stats, error) {
	var tracks []TrackInfo
	var stats Stats

	fmt.Printf("Scanning: %s\n", folder)

	err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !supportedExtensions[ext] {
			return nil
		}

		stats.TotalFiles++

		// Show progress
		rel, _ := filepath.Rel(folder, path)
		fmt.Printf("\r  Scanning: %-60s", truncateString(rel, 60))

		track, err := extractMetadata(path)
		if err != nil {
			stats.Errors++
			return nil
		}

		if track.HasAllTags(appConfig.Tags) {
			stats.CompleteFiles++
		} else {
			stats.MissingFiles++
			tracks = append(tracks, track)
		}

		return nil
	})

	fmt.Printf("\r%80s\r", "") // Clear the line

	return tracks, stats, err
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return "..." + s[len(s)-maxLen+3:]
}

// extractMetadata uses ffprobe to extract tags from an audio file
func extractMetadata(path string) (TrackInfo, error) {
	track := TrackInfo{
		Path: path,
		Tags: make(map[string]string),
	}

	// Use ffprobe to get tags
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "flat",
		"-show_entries", "format_tags",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return track, fmt.Errorf("ffprobe failed: %w", err)
	}

	// Parse ffprobe output
	// Format: format.tags.tag_name="value"
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "format.tags.") {
			continue
		}

		// Extract key=value
		line = strings.TrimPrefix(line, "format.tags.")
		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.ToUpper(line[:idx])
		value := strings.Trim(line[idx+1:], "\"")

		// Store in tags map
		track.Tags[key] = value

		// Also set convenience fields
		switch key {
		case "ARTIST":
			track.Artist = value
		case "ALBUM":
			track.Album = value
		case "TITLE":
			track.Title = value
		}
	}

	// Use filename as fallback for title
	if track.Title == "" {
		track.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if track.Artist == "" {
		track.Artist = "Unknown Artist"
	}
	if track.Album == "" {
		track.Album = "Unknown Album"
	}

	return track, nil
}

// writeReport writes the missing tags report to a file
func writeReport(tracks []TrackInfo, outputPath string, config Config) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	if config.Format == "artist" {
		writeArtistFormat(writer, tracks, config.Tags)
	} else {
		writeAlbumFormat(writer, tracks, config.Tags)
	}

	return writer.Flush()
}

// writeAlbumFormat writes in format:
// Album:
//
//	Song [missing: ...]
func writeAlbumFormat(writer *bufio.Writer, tracks []TrackInfo, requiredTags []string) {
	currentAlbum := ""

	for _, track := range tracks {
		// Print album header when it changes
		if track.Album != currentAlbum {
			if currentAlbum != "" {
				fmt.Fprintln(writer) // Blank line between albums
			}
			fmt.Fprintf(writer, "%s:\n", track.Album)
			currentAlbum = track.Album
		}

		// Print track with missing tags
		missingTags := track.MissingTags(requiredTags)
		fmt.Fprintf(writer, "    %s [missing: %s]\n",
			track.Title, strings.Join(missingTags, ", "))
	}
}

// writeArtistFormat writes in format:
// Artist:
//
//	Album:
//	    Song [missing: ...]
func writeArtistFormat(writer *bufio.Writer, tracks []TrackInfo, requiredTags []string) {
	currentArtist := ""
	currentAlbum := ""

	for _, track := range tracks {
		// Print artist header when it changes
		if track.Artist != currentArtist {
			if currentArtist != "" {
				fmt.Fprintln(writer) // Blank line between artists
			}
			fmt.Fprintf(writer, "%s:\n", track.Artist)
			currentArtist = track.Artist
			currentAlbum = "" // Reset album when artist changes
		}

		// Print album header when it changes
		if track.Album != currentAlbum {
			fmt.Fprintf(writer, "    %s:\n", track.Album)
			currentAlbum = track.Album
		}

		// Print track with missing tags
		missingTags := track.MissingTags(requiredTags)
		fmt.Fprintf(writer, "        %s [missing: %s]\n",
			track.Title, strings.Join(missingTags, ", "))
	}
}
