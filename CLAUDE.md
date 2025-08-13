# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go application that scrapes websites and creates fully self-contained static HTML copies with all assets localized. The tool has two main modes:
1. **Scrape mode**: Downloads CSS, JavaScript, images, and font files from a given URL, saves them locally with auto-cleanup of previous files, and updates HTML to reference local copies
2. **Serve mode**: Starts an HTTP server to serve the scraped content locally for immediate preview with proper asset routing

Key improvements include:
- Advanced font discovery and localization (including protocol-relative URLs)
- Smart asset detection for absolute and relative paths
- Responsive image support (srcset attribute processing)
- Inline CSS font processing within `<style>` tags
- Preload link handling with proper href localization
- Source map reference removal to prevent browser errors
- Error suppression for development server connections and security errors
- Automatic cleanup before each scrape

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

The application is organized into modular packages for maintainability and clarity:

### Core Packages

**`main.go`**: Entry point that routes to commands based on first argument
- `main()`: Command routing logic

**`commands/`**: Command handlers and user interface
- `scrape.go`: `ScrapeCommand()` - Handles the scraping workflow with auto-cleanup, URL and output file flags
- `serve.go`: `ServeCommand()` - Starts HTTP server with proper routing for assets and fonts
- `usage.go`: `PrintUsage()` - Displays help information for available commands

**`assets/`**: Asset downloading and processing logic
- `downloader.go`: `DownloadResource()`, `DownloadImage()` - Downloads external resources and saves them to the `output/assets/` directory
- `processor.go`: `LocalizeAssets()` - Parses HTML and processes `<link>`, `<script>`, `<img>`, and `<meta>` tags to localize all assets
  - `LocalizeFontURLs()`: Advanced font discovery that processes both absolute URLs, relative paths, and protocol-relative URLs
  - `LocalizeSrcset()`: Processes responsive image srcset attributes with multiple image URLs and descriptors
  - `LocalizeStyleBackgroundImages()`: Handles background images in inline style attributes
  - `LocalizeJavaScriptURLs()`: Processes JavaScript content for embedded resource URLs

**`html/`**: HTML processing utilities
- `processor.go`: `AddErrorSuppressionScript()` - Injects JavaScript to suppress development server and security errors

**`utils/`**: Shared utility functions
- `cleanup.go`: `CleanupOldFiles()`, `EnsureDirectories()` - Removes previous output directory and creates necessary directories
- `url.go`: `ResolveURL()` - Resolves relative URLs against the base URL
- Source map processing: `RemoveSourceMapReferences()` - Strips source map comments from CSS and JS files

## File Structure

### Source Code Organization
- `main.go`: Entry point with command routing
- `commands/`: Command handlers (scrape.go, serve.go, usage.go)
- `assets/`: Asset downloading and processing (downloader.go, processor.go)
- `html/`: HTML processing utilities (processor.go)
- `utils/`: Shared utilities (cleanup.go, url.go)
- `wp-static-scraper`: Compiled binary

### Runtime Output Structure
- `output/`: Root directory for all scraped content
- `output/index.html`: Default output file (configurable via `-out` flag)
- `output/assets/`: Directory containing downloaded CSS, JavaScript, and other assets
- `output/assets/fonts/`: Subdirectory containing all downloaded font files (TTF, WOFF, WOFF2, EOT, SVG)
- `output/assets/images/`: Subdirectory containing all downloaded images (PNG, JPG, GIF, WebP, SVG)

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

### Core Assets
1. **CSS stylesheets** (`<link rel="stylesheet">` and `<link rel="preload">`) - Downloaded to `assets/`
2. **JavaScript files** (`<script src="">`) - Downloaded to `assets/`
3. **Images** (`<img src="">`, `<img srcset="">`, meta tags, background images) - Downloaded to `assets/images/`

### Advanced Font Discovery
4. **Font files** - Comprehensive detection and download to `assets/fonts/`:
   - Absolute URLs: `url(https://example.com/font.woff2)`
   - Protocol-relative URLs: `url(//example.com/font.woff2)`
   - Relative paths: `url(../webfonts/fa-regular-400.woff2)`
   - Inline CSS within `<style>` tags
   - Multiple formats: TTF, WOFF, WOFF2, EOT, SVG
   - FontAwesome support: Automatically discovers all FontAwesome variants

### Image Processing
5. **Responsive images** - Handles modern web image techniques:
   - `srcset` attribute processing with size descriptors (e.g., `image.jpg 300w`)
   - Background images in inline `style` attributes
   - Meta tag images (og:image, twitter:image, etc.)
   - Lazy loading images (`data-src` attributes)

### Error Prevention
6. **Development artifacts removal**:
   - Source map references (`sourceMappingURL`) stripped from CSS/JS
   - Error suppression for localhost development servers
   - Security error suppression for service worker origin mismatches
   - Network error handling for failed connections

### Server Routing
The HTTP server provides proper routing:
- `/assets/` - Serves CSS, JS, fonts, and images (maps to `output/assets/`)
- `/webfonts/` - Alternative path for FontAwesome fonts (maps to `output/assets/fonts/`)
- `/fonts/` - Direct font access (maps to `output/assets/fonts/`)
- `/images/` - Direct image access (maps to `output/assets/images/`)
- `/` - Serves the main HTML file from `output/index.html`

### Process Flow
1. **Cleanup**: Remove previous `output/` directory and all its contents
2. **Setup**: Create `output/assets/`, `output/assets/images/`, and `output/assets/fonts/` directories
3. **Download**: Fetch HTML and parse for all asset references
4. **Localize**: Download all assets to `output/assets/` and update HTML references
5. **Process**: Remove source maps, process fonts, handle responsive images
6. **Enhance**: Inject error suppression script
7. **Save**: Write final HTML to `output/index.html` (or custom filename)
8. **Serve**: HTTP server with proper MIME types and routing for the `output/` directory

All assets are downloaded and stored locally in the `output/` directory with HTML updated to reference local copies, ensuring fully offline-capable static sites with no external dependencies.