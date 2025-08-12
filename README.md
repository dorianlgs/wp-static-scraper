# wp-static-scraper

A Go application that scrapes websites and creates fully self-contained static HTML copies with all assets (CSS, JavaScript, fonts) downloaded and localized.

## Features

- Downloads and saves complete web pages as static HTML
- Localizes all external CSS stylesheets
- Localizes all external JavaScript files
- **Advanced font discovery**: Automatically downloads and localizes all font files referenced in CSS (including FontAwesome and other web fonts)
- **Smart asset detection**: Finds both absolute URLs and relative paths in CSS files
- **Built-in HTTP server**: Serves scraped content locally for immediate preview
- **Auto-cleanup**: Automatically removes old files before each scrape for clean results
- Creates organized directory structure with fonts stored in `assets/fonts/`
- Preserves original HTML structure while updating asset references

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

**Serve command:**
- `-port`: (Optional) Port for HTTP server (default: 8080)

## Output Structure

When you run the scraper, it creates:
- The main HTML file (default: `index.html`)
- `assets/` directory containing downloaded CSS and JavaScript files
- `assets/fonts/` directory containing all downloaded font files (TTF, WOFF, WOFF2, EOT, SVG formats)

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
index.html
assets/
  ├── all.min.css
  ├── style.css
  ├── main.js
  ├── other-assets...
  └── fonts/
      ├── fa-brands-400.woff2
      ├── fa-regular-400.woff2
      ├── fa-solid-900.woff2
      ├── font1.ttf
      └── other-fonts...
```

## Key Features

### Comprehensive Font Support
The scraper intelligently discovers and downloads all font formats referenced in CSS files:
- **FontAwesome**: Automatically detects and downloads all FontAwesome font variants
- **Google Fonts**: Downloads custom fonts from Google Fonts and other CDNs
- **Multiple formats**: Supports TTF, WOFF, WOFF2, EOT, and SVG font formats
- **Relative paths**: Handles both absolute URLs and relative paths in CSS (e.g., `../webfonts/`)

### Smart Asset Detection
- Parses CSS files to find all `url()` references
- Resolves relative paths against the original website's base URL
- Updates CSS to use local font paths for offline viewing

### Clean Workflow
- Each scrape automatically removes previous assets and HTML files
- Prevents mixing assets from different websites
- Ensures fresh, clean results every time

## Requirements

- Go 1.24.0 or later
- Internet connection for downloading assets