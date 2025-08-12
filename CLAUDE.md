# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go application that scrapes websites and creates fully self-contained static HTML copies with all assets localized. The tool has two main modes:
1. **Scrape mode**: Downloads CSS, JavaScript, and font files from a given URL, saves them locally with auto-cleanup of previous files, and updates HTML to reference local copies
2. **Serve mode**: Starts an HTTP server to serve the scraped content locally for immediate preview with proper asset routing

Key improvements include advanced font discovery, smart asset detection for both absolute and relative paths, and automatic cleanup before each scrape.

## Development Commands

```bash
# Build the application
go build -o wp-static-scraper main.go

# Run the application (scraping)
go run main.go scrape -url "https://example.com"

# Run the application (server mode)
go run main.go serve

# Format code
go fmt ./...

# Run tests (if any exist)
go test ./...

# Install dependencies
go mod tidy
```

## Architecture

The application consists of a single `main.go` file with the following key functions:

- `main()`: Entry point that routes to commands based on first argument
- `printUsage()`: Displays help information for available commands
- `cleanupOldFiles()`: Removes previous assets directory and output HTML file before scraping
- `scrapeCommand()`: Handles the scraping workflow with auto-cleanup, URL and output file flags
- `serveCommand()`: Starts HTTP server with proper routing for assets and fonts
- `localizeAssets()`: Parses HTML and processes `<link>` and `<script>` tags to localize CSS and JS files
- `downloadResource()`: Downloads external resources and saves them to the `assets/` directory, processes CSS for fonts
- `localizeFontURLs()`: Advanced font discovery that processes both absolute URLs and relative paths in CSS files
- `resolveURL()`: Resolves relative URLs against the base URL

## File Structure

- `main.go`: Main application code
- `assets/`: Directory created at runtime to store downloaded CSS and JavaScript files
- `assets/fonts/`: Subdirectory containing all downloaded font files (TTF, WOFF, WOFF2, EOT, SVG)
- `index.html`: Default output file (configurable via `-out` flag)
- `wp-static-scraper`: Compiled binary

## Key Dependencies

- `golang.org/x/net/html`: HTML parsing and manipulation
- Standard library packages for HTTP, URL parsing, file I/O, and regex

## CLI Usage

The application supports two main commands:

**Scrape command:**
- `./wp-static-scraper scrape -url <URL> [-out <filename>]`
- `-url`: Required. The URL of the website to scrape
- `-out`: Optional. Output HTML file path (defaults to "index.html")

**Serve command:**
- `./wp-static-scraper serve [-port <port>]`
- `-port`: Optional. Port for HTTP server (defaults to 8080)

## Asset Handling

The scraper provides comprehensive asset detection and localization:

### Primary Assets
1. **CSS stylesheets** (`<link rel="stylesheet">`) - Downloaded to `assets/`
2. **JavaScript files** (`<script src="">`) - Downloaded to `assets/`

### Advanced Font Discovery
3. **Font files** - Comprehensive detection and download to `assets/fonts/`:
   - Absolute URLs: `url(https://example.com/font.woff2)`
   - Relative paths: `url(../webfonts/fa-regular-400.woff2)`
   - Multiple formats: TTF, WOFF, WOFF2, EOT, SVG
   - FontAwesome support: Automatically discovers all FontAwesome variants

### Server Routing
The HTTP server provides proper routing:
- `/assets/` - Serves CSS, JS and fonts
- `/webfonts/` - Alternative path for FontAwesome fonts (maps to `assets/fonts/`)

### Process Flow
1. **Cleanup**: Remove previous `assets/` directory and output HTML file
2. **Download**: Fetch HTML and parse for asset references
3. **Localize**: Download all assets and update HTML references
4. **Serve**: HTTP server with proper MIME types and routing

All assets are downloaded and stored locally with HTML updated to reference local copies, ensuring fully offline-capable static sites.