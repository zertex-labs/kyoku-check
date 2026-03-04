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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/zertex-labs/kyoku-check/ui"
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
	model := ui.NewScanModel(absFolder)
	p := tea.NewProgram(model)

	// Start the scan in a goroutine
	go runScanWithUI(absFolder, p)

	finalModel, err := p.Run()
	if err != nil {
		// Fallback to non-interactive mode
		tracks, stats, err = runScanSimple(absFolder)
		if err != nil {
			return err
		}
	} else {
		m := finalModel.(*ui.ScanModel)
		if m.IsCancelled() {
			return fmt.Errorf("scan cancelled")
		}

		// Get stats from model
		stats.TotalFiles, stats.CompleteFiles, stats.MissingFiles, stats.Errors = m.GetStats()

		// Get missing files from model
		for _, f := range m.GetMissingFiles() {
			tracks = append(tracks, TrackInfo{
				Path:   f.Path,
				Artist: f.Artist,
				Album:  f.Album,
				Title:  f.Title,
				Tags:   make(map[string]string), // We don't have full tags, but have missing list
			})
			// Store missing tags for later (hacky but works)
			tracks[len(tracks)-1].Tags["_MISSING"] = strings.Join(f.MissingTags, ",")
		}
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

	// Write output file if there are missing files, or if no_missing_text is configured
	if len(tracks) > 0 {
		if err := writeReport(tracks, outputFile, appConfig); err != nil {
			return fmt.Errorf("failed to write report: %w", err)
		}
	} else if appConfig.NoMissingText != "" {
		if err := writeNoMissingReport(outputFile, appConfig.NoMissingText); err != nil {
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

	if len(tracks) > 0 || appConfig.NoMissingText != "" {
		absPath, _ := filepath.Abs(outputFile)
		fmt.Printf("\nReport written to: %s\n", absPath)
	} else {
		fmt.Println("\nAll files have complete tags!")
	}

	return nil
}

// runScanWithUI runs the scan with UI updates
func runScanWithUI(folder string, p *tea.Program) {
	// First, collect all files
	var files []string
	filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if supportedExtensions[ext] {
			files = append(files, path)
		}
		return nil
	})

	// Send total count
	p.Send(ui.FileFoundMsg{Total: len(files)})

	// Process files in parallel
	parallelCount := appConfig.ParallelCount
	if parallelCount < 1 {
		parallelCount = 1
	}

	// Create worker pool
	type job struct {
		index int
		path  string
	}

	jobs := make(chan job, len(files))
	var wg sync.WaitGroup

	// Start workers
	for w := 0; w < parallelCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Signal start
				p.Send(ui.FileStartMsg{Index: j.index})

				// Extract metadata
				track, err := extractMetadata(j.path)
				if err != nil {
					p.Send(ui.FileErrorMsg{Index: j.index, Error: err.Error()})
					continue
				}

				// Check for missing tags
				missingTags := track.MissingTags(appConfig.Tags)
				p.Send(ui.FileCompleteMsg{
					Index:       j.index,
					MissingTags: missingTags,
					Album:       track.Album,
					Artist:      track.Artist,
					Title:       track.Title,
				})
			}
		}()
	}

	// Send jobs
	for i, path := range files {
		jobs <- job{index: i, path: path}
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()

	// Signal completion
	p.Send(ui.ScanCompleteMsg{})
}

// runScanSimple runs the scan without TUI (fallback for non-TTY environments)
func runScanSimple(folder string) ([]TrackInfo, Stats, error) {
	var tracks []TrackInfo
	var stats Stats

	fmt.Printf("Scanning: %s\n", folder)

	// First collect all files
	var files []string
	filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if supportedExtensions[ext] {
			files = append(files, path)
		}
		return nil
	})

	stats.TotalFiles = len(files)

	// Process with parallel workers
	parallelCount := appConfig.ParallelCount
	if parallelCount < 1 {
		parallelCount = 1
	}

	type result struct {
		track TrackInfo
		err   error
	}

	jobs := make(chan string, len(files))
	results := make(chan result, len(files))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < parallelCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				track, err := extractMetadata(path)
				results <- result{track: track, err: err}
			}
		}()
	}

	// Send jobs
	for _, path := range files {
		jobs <- path
	}
	close(jobs)

	// Close results when done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	processed := 0
	for r := range results {
		processed++
		fmt.Printf("\r  Processing: %d/%d files", processed, stats.TotalFiles)

		if r.err != nil {
			stats.Errors++
			continue
		}

		if r.track.HasAllTags(appConfig.Tags) {
			stats.CompleteFiles++
		} else {
			stats.MissingFiles++
			tracks = append(tracks, r.track)
		}
	}

	fmt.Printf("\r%80s\r", "") // Clear the line

	return tracks, stats, nil
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
		var missingTags []string
		if stored, ok := track.Tags["_MISSING"]; ok && stored != "" {
			missingTags = strings.Split(stored, ",")
		} else {
			missingTags = track.MissingTags(requiredTags)
		}
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
		var missingTags []string
		if stored, ok := track.Tags["_MISSING"]; ok && stored != "" {
			missingTags = strings.Split(stored, ",")
		} else {
			missingTags = track.MissingTags(requiredTags)
		}
		fmt.Fprintf(writer, "        %s [missing: %s]\n",
			track.Title, strings.Join(missingTags, ", "))
	}
}

// writeNoMissingReport writes the no_missing_text to the output file
func writeNoMissingReport(outputPath string, text string) error {
	// Replace %checked_at% with current timestamp
	text = strings.ReplaceAll(text, "%checked_at%", time.Now().Format("02-01-2006 15:04"))
	return os.WriteFile(outputPath, []byte(text+"\n"), 0644)
}
