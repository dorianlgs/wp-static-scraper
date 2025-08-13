# wp-static-scraper

A Go application that scrapes websites and creates fully self-contained static HTML copies with all assets (CSS, JavaScript, images, fonts) downloaded and localized for completely offline viewing.

## Features

- **High-performance concurrent downloads**: Optimized worker pool with HTTP connection pooling for 50%+ faster scraping
- **Complete website capture**: Downloads and saves web pages as fully self-contained static HTML
- **CSS & JavaScript**: Localizes all external stylesheets and scripts (including preload links)
- **Image processing**: Downloads all images including responsive `srcset` variants, background images, and meta tag images
- **Advanced font discovery**: Automatically downloads font files from CSS, including:
  - Protocol-relative URLs (`//domain.com/font.woff2`)
  - Inline CSS within `<style>` tags
  - FontAwesome and Google Fonts
  - All formats: TTF, WOFF, WOFF2, EOT, SVG
- **Smart asset detection**: Finds both absolute URLs and relative paths in CSS files
- **True parallelism**: All asset types (CSS, JS, images, fonts) download simultaneously
- **Error prevention**: 
  - Removes source map references to prevent browser errors
  - Suppresses development server connection errors
  - Handles security errors from service worker origin mismatches
- **Built-in HTTP server**: Serves scraped content locally with proper routing
- **Auto-cleanup**: Automatically removes old files before each scrape
- **Organized structure**: Creates clean directory structure with separate folders for different asset types

## Installation

```bash
go build -o wp-static-scraper main.go
```

## Usage

The application supports two main commands: `scrape` for downloading websites and `serve` for serving the scraped content.

### Scraping Websites

```bash
# Basic scraping
./wp-static-scraper scrape -url "https://example.com"

# Specify output file
./wp-static-scraper scrape -url "https://example.com" -out "my-page.html"

# High-performance scraping with custom concurrency
./wp-static-scraper scrape -url "https://example.com" -concurrency 50
```

### Serving Scraped Content

```bash
# Start server on default port (8080)
./wp-static-scraper serve

# Start server on custom port
./wp-static-scraper serve -port 3000
```

### Command Line Options

**Scrape command:**
- `-url`: (Required) The URL of the website to scrape
- `-out`: (Optional) Output HTML file path (default: "index.html")
- `-concurrency`: (Optional) Number of concurrent download workers, 1-100 (default: 100)

**Serve command:**
- `-port`: (Optional) Port for HTTP server (default: 8080)

## Output Structure

When you run the scraper, it creates an organized `output/` directory:
- `output/index.html` (or custom filename) with all references updated to local assets
- `output/assets/` directory containing downloaded CSS and JavaScript files
- `output/assets/fonts/` directory containing all downloaded font files (TTF, WOFF, WOFF2, EOT, SVG formats)
- `output/assets/images/` directory containing all downloaded images (PNG, JPG, GIF, WebP, SVG formats)

## Example Workflow

```bash
# 1. Scrape a website
./wp-static-scraper scrape -url "https://example.com"

# 2. Start local server to view the scraped content
./wp-static-scraper serve

# 3. Open http://localhost:8080 in your browser
```

This creates the following structure:
```
output/
├── index.html
└── assets/
    ├── all.min.css
    ├── style.css
    ├── main.js
    ├── other-scripts.js
    ├── fonts/
    │   ├── fa-brands-400.woff2
    │   ├── fa-regular-400.woff2
    │   ├── fa-solid-900.woff2
    │   ├── Montserrat-Regular.woff2
    │   └── other-fonts...
    └── images/
        ├── logo.png
        ├── hero-image.jpg
        ├── banner-mobile.webp
        ├── icon.svg
        └── other-images...
```

## Key Features

### Comprehensive Asset Support
The scraper intelligently discovers and downloads all assets referenced in web pages:

**Fonts:**
- **FontAwesome**: Automatically detects and downloads all FontAwesome font variants
- **Google Fonts**: Downloads custom fonts from Google Fonts and other CDNs
- **Protocol-relative URLs**: Handles `//domain.com/font.woff2` references
- **Inline CSS**: Processes fonts in `<style>` tags
- **Multiple formats**: Supports TTF, WOFF, WOFF2, EOT, and SVG font formats
- **Relative paths**: Handles both absolute URLs and relative paths (e.g., `../webfonts/`)

**Images:**
- **Responsive images**: Processes `srcset` attributes with size descriptors
- **Background images**: Extracts images from inline `style` attributes
- **Meta images**: Downloads og:image, twitter:image, and other meta tag images
- **Lazy loading**: Handles `data-src` attributes for deferred loading
- **All formats**: PNG, JPG, GIF, WebP, SVG, and more

**Scripts & Styles:**
- **Preload links**: Properly handles `<link rel="preload">` tags
- **Source maps**: Removes `sourceMappingURL` references to prevent errors
- **Error suppression**: Injects scripts to handle development server errors

### Smart Asset Detection
- Parses HTML, CSS, and JavaScript to find all asset references
- Resolves relative paths against the original website's base URL
- Updates all references to use local paths for offline viewing
- Handles complex CSS with nested imports and font-face declarations

### Clean Workflow
- Each scrape automatically removes previous assets and HTML files
- Prevents mixing assets from different websites
- Ensures fresh, clean results every time
- Creates organized directory structure for easy navigation

## Performance

The scraper uses an optimized concurrent download system for maximum performance:

- **True parallelism**: All asset types download simultaneously (not in sequential phases)
- **HTTP connection pooling**: Reuses connections for better network efficiency  
- **Optimized worker pool**: Simple job queue with atomic counters eliminates bottlenecks
- **Non-blocking retries**: Failed downloads retry asynchronously without blocking workers
- **Upfront asset discovery**: Finds all assets including fonts from inline CSS immediately

**Benchmark**: 53% performance improvement (10s → 4.7s) on complex websites with 50 concurrent workers.

## Architecture

The application is built with a modular package structure for maintainability and clarity:

- **`main.go`**: Entry point with command routing
- **`commands/`**: Command handlers for scrape, serve, and usage
- **`assets/`**: High-performance asset downloading with concurrent worker pool
- **`html/`**: HTML processing and error suppression utilities
- **`utils/`**: Shared utilities for cleanup and URL resolution
- **`output/`**: Generated directory containing scraped content

## Requirements

- Go 1.24.0 or later
- Internet connection for downloading assets