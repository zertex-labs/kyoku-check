# kyoku-check

A command-line tool that scans music libraries and identifies audio files missing metadata tags(defined in the configuration file; by default main MusicBrainz tags and genre)

## Features

- Recursively scans directories for audio files
- Supports multiple formats: FLAC, MP3, M4A, OGG, and Opus
- Checks for configurable metadata tags (MusicBrainz IDs, genre, etc.)
- Generates organized reports grouped by album or artist
- Falls back to simple text output in non-TTY environments

## Prerequisites

- Go 1.25.6 or later
- FFmpeg (`ffprobe` must be installed and available in PATH)

## Installation

```bash
# Clone and build
git clone https://github.com/zertex-labs/kyoku-check.git
cd kyoku-check
go build -o kyoku-check

# Or install directly
go install github.com/zertex-labs/kyoku-check@latest
```

## Usage

```bash
# Scan a music directory
kyoku-check /path/to/music

# Specify a custom output file
kyoku-check -o report.txt /path/to/music

# Use a custom config file
kyoku-check --config /path/to/config.json /path/to/music
```

### CLI Flags

| Flag       | Short | Default              | Description                  |
|------------|-------|----------------------|------------------------------|
| `--config` |       | `~/.config/kyoku-check/config.json` | Custom config file path |
| `--output` | `-o`  | `missing-tags.txt`   | Output file for the report   |

## Configuration

The configuration file is located at `~/.config/kyoku-check/config.json` (or `$XDG_CONFIG_HOME/kyoku-check/config.json`). A default config is created automatically on first run.

### Config File Format

```json
{
  "tags": [
    "MUSICBRAINZ_TRACKID",
    "MUSICBRAINZ_ALBUMID",
    "MUSICBRAINZ_ARTISTID",
    "genre"
  ],
  "format": "album"
}
```

### Configuration Options

| Option   | Type       | Default | Description |
|----------|------------|---------|-------------|
| `tags`   | `[]string` | `["MUSICBRAINZ_TRACKID", "MUSICBRAINZ_ALBUMID", "MUSICBRAINZ_ARTISTID", "genre"]` | List of metadata tags to check for. Tag names are case-insensitive. |
| `format` | `string`   | `"album"` | Output format for the report. Options: `"album"` or `"artist"`. |

### Output Formats

**Album format** (default; recommended) - Groups files by album:

```
Album Name:
    Song Title [missing: MUSICBRAINZ_TRACKID, genre]
    Another Song [missing: MUSICBRAINZ_ALBUMID]

Another Album:
    Track [missing: genre]
```

**Artist format** - Groups files by artist, then album:

```
Artist Name:
    Album Name:
        Song Title [missing: MUSICBRAINZ_TRACKID, genre]
        Another Song [missing: MUSICBRAINZ_ALBUMID]

Another Artist:
    Their Album:
        Track [missing: genre]
```

## Supported Audio Formats

- `.flac` - Free Lossless Audio Codec
- `.mp3` - MPEG Audio Layer III
- `.m4a` - MPEG-4 Audio
- `.ogg` - Ogg Vorbis
- `.opus` - Opus Audio

## License

MIT
