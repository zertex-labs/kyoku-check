package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	outputFile string
	cfgFile    string
	appConfig  Config
)

// Config holds the application configuration
type Config struct {
	// Tags to check for (e.g., ["MUSICBRAINZ_TRACKID", "MUSICBRAINZ_ALBUMID", "genre"])
	Tags []string `json:"tags"`
	// Format for the output file: "album" or "artist"
	// "album" = Album (Artist):\n    Song
	// "artist" = Artist:\n    Album:\n        Song
	Format string `json:"format"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		Tags: []string{
			"MUSICBRAINZ_TRACKID",
			"MUSICBRAINZ_ALBUMID",
			"MUSICBRAINZ_ARTISTID",
			"genre",
		},
		Format: "album",
	}
}

var rootCmd = &cobra.Command{
	Use:   "kyoku-check <folder>",
	Short: "Check music files for MusicBrainz tags",
	Long: `kyoku-check scans a folder for music files and checks if they have
the configured metadata tags.

Files missing tags are written to an output file. The format can be configured:
  - "album" format (default):
      Album (Artist):
          Song [missing: ...]
  - "artist" format:
      Artist:
          Album:
              Song [missing: ...]

Configuration file: ~/.config/kyoku-check/config.json`,
	Args: cobra.ExactArgs(1),
	RunE: runCheck,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/kyoku-check/config.json)")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "missing-tags.txt", "output file for missing tags report")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// initConfig reads the config file
func initConfig() {
	if cfgFile == "" {
		cfgFile = filepath.Join(defaultConfigDir(), "config.json")
	}

	appConfig = loadConfig(cfgFile)
}

// defaultConfigDir returns the default configuration directory
func defaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kyoku-check")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "kyoku-check")
}

// loadConfig reads the config file and returns the configuration
func loadConfig(path string) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create config file with defaults
			if err := createDefaultConfig(path); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create default config: %v\n", err)
			}
		}
		return DefaultConfig()
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse config: %v\n", err)
		return DefaultConfig()
	}

	// Validate format
	if config.Format != "album" && config.Format != "artist" {
		config.Format = "album"
	}

	// Ensure tags has at least one entry
	if len(config.Tags) == 0 {
		config.Tags = DefaultConfig().Tags
	}

	return config
}

// createDefaultConfig creates a config file with default values
func createDefaultConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	config := DefaultConfig()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created default config at %s\n", path)
	return nil
}
