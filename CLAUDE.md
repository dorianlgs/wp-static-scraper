# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go application that scrapes websites and creates fully self-contained static HTML copies with all assets localized. The tool has two main modes:
1. **Scrape mode**: Downloads CSS, JavaScript, images, and font files from a given URL, saves them locally with auto-cleanup of previous files, and updates HTML to reference local copies
2. **Serve mode**: Starts an HTTP server to serve the scraped content locally for immediate preview with proper asset routing

Key improvements include:
- **High-performance concurrent downloads**: True parallelism with optimized worker pool and HTTP connection pooling
- **Advanced font discovery and localization**: Including protocol-relative URLs and inline CSS processing
- **Smart asset detection**: For absolute and relative paths across all asset types
- **Responsive image support**: srcset attribute processing with size descriptors
- **Inline CSS font processing**: Within `<style>` tags for comprehensive font coverage
- **Preload link handling**: With proper href localization
- **Source map reference removal**: To prevent browser errors
- **Error suppression**: For development server connections and security errors
- **Automatic cleanup**: Before each scrape for clean results

## Development Commands

```bash
# Build the application
go build -ldflags "-s -w" -o wp-static-scraper main.go

# Run the application (scraping)
go run main.go scrape -url "https://example.com"

# Run with custom concurrency (1-100 workers)
go run main.go scrape -url "https://example.com" -concurrency 50

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

**`assets/`**: High-performance asset downloading and processing logic
- `concurrent.go`: `ConcurrentDownloader` - Optimized worker pool with HTTP connection pooling, atomic counters, and non-blocking retries
- `downloader.go`: `DownloadResource()`, `DownloadImage()` - Legacy download functions (now integrated into concurrent system)
- `processor.go`: `LocalizeAssets()` - Parses HTML and processes all asset types with true parallelism
  - `collectAllAssetJobs()`: Upfront discovery of all assets including fonts from inline CSS
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
- `assets/`: Asset downloading and processing (concurrent.go, downloader.go, processor.go)
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
- `./wp-static-scraper scrape -url <URL> [-out <filename>] [-concurrency <workers>]`
- `-url`: Required. The URL of the website to scrape
- `-out`: Optional. Output HTML file path (defaults to "index.html")
- `-concurrency`: Optional. Number of concurrent download workers, 1-100 (defaults to 100)

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
3. **Discovery**: Parse HTML and collect ALL asset URLs upfront (CSS, JS, images, fonts from inline CSS)
4. **Parallel Download**: Download all assets simultaneously using optimized worker pool with HTTP connection pooling
5. **Process**: Remove source maps, handle responsive images, process JavaScript templates
6. **Enhance**: Inject error suppression script
7. **Save**: Write final HTML to `output/index.html` (or custom filename)
8. **Serve**: HTTP server with proper MIME types and routing for the `output/` directory

All assets are downloaded and stored locally in the `output/` directory with HTML updated to reference local copies, ensuring fully offline-capable static sites with no external dependencies.

## Performance

The concurrent download system provides significant performance improvements:
- **True parallelism**: All asset types (CSS, JS, images, fonts) download simultaneously
- **HTTP connection pooling**: Reuses connections for better network efficiency
- **Optimized worker pool**: Simple job queue with atomic counters eliminates mutex contention
- **Non-blocking retries**: Failed downloads retry asynchronously without blocking workers
- **Upfront asset discovery**: Finds all assets including fonts from inline CSS immediately

**Benchmark**: 53% performance improvement (10s → 4.7s) on complex websites with 50 concurrent workers.