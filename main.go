package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "scrape":
		scrapeCommand()
	case "serve":
		serveCommand()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("wp-static-scraper - Web scraper with local server")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  wp-static-scraper scrape -url <URL> [-out <filename>]")
	fmt.Println("  wp-static-scraper serve [-port <port>]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  scrape    Download and localize a website")
	fmt.Println("  serve     Start HTTP server to serve scraped content")
	fmt.Println("")
	fmt.Println("Scrape options:")
	fmt.Println("  -url      URL of the website to scrape (required)")
	fmt.Println("  -out      Output HTML file (default: index.html)")
	fmt.Println("")
	fmt.Println("Serve options:")
	fmt.Println("  -port     Port for HTTP server (default: 8080)")
}

func cleanupOldFiles(outputFile string) {
	// Remove assets directory and all its contents (includes images, fonts, etc.)
	os.RemoveAll("assets")

	// Remove the output HTML file
	os.Remove(outputFile)
}

func scrapeCommand() {
	scrapeFlags := flag.NewFlagSet("scrape", flag.ExitOnError)
	inputURL := scrapeFlags.String("url", "", "URL of the website to scrape")
	outputFile := scrapeFlags.String("out", "index.html", "Output HTML file")
	scrapeFlags.Parse(os.Args[2:])

	if *inputURL == "" {
		fmt.Println("Please provide a URL with -url flag.")
		scrapeFlags.Usage()
		os.Exit(1)
	}

	// Clean up old files before starting new scrape
	cleanupOldFiles(*outputFile)

	resp, err := http.Get(*inputURL)
	if err != nil {
		fmt.Printf("Failed to fetch URL: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response body: %v\n", err)
		os.Exit(1)
	}

	base, err := url.Parse(*inputURL)
	if err != nil {
		fmt.Printf("Invalid base URL: %v\n", err)
		os.Exit(1)
	}

	os.MkdirAll("assets", 0755)
	os.MkdirAll("assets/images", 0755)

	updatedHTML, err := localizeAssets(string(body), base)
	if err != nil {
		fmt.Printf("Failed to localize assets: %v\n", err)
		os.Exit(1)
	}

	// Add script to suppress localhost development server errors
	updatedHTML = addErrorSuppressionScript(updatedHTML)

	err = os.WriteFile(*outputFile, []byte(updatedHTML), 0644)
	if err != nil {
		fmt.Printf("Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Static HTML with local assets saved to %s\n", *outputFile)
}

func serveCommand() {
	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	port := serveFlags.Int("port", 8080, "Port for HTTP server")
	serveFlags.Parse(os.Args[2:])

	// Check if index.html exists
	if _, err := os.Stat("index.html"); os.IsNotExist(err) {
		fmt.Println("index.html not found. Please run 'scrape' command first.")
		os.Exit(1)
	}

	// Set up file server for static assets
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Handle direct /webfonts/ requests (for CSS files that reference absolute webfonts paths)
	http.Handle("/webfonts/", http.StripPrefix("/webfonts/", http.FileServer(http.Dir("assets/fonts"))))

	// Handle direct /fonts/ requests (for CSS files that reference fonts/ paths)
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("assets/fonts"))))

	// Handle direct /images/ requests for downloaded images
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("assets/images"))))

	// Serve index.html at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "index.html")
		} else {
			http.NotFound(w, r)
		}
	})

	fmt.Printf("Starting server on http://localhost:%d\n", *port)
	fmt.Println("Press Ctrl+C to stop the server")
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}

func localizeAssets(htmlContent string, base *url.URL) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	var b strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			isStylesheet := false
			isPreload := false
			var href string
			for _, attr := range n.Attr {
				if attr.Key == "rel" {
					if attr.Val == "stylesheet" {
						isStylesheet = true
					} else if attr.Val == "preload" {
						isPreload = true
					}
				}
				if attr.Key == "href" {
					href = attr.Val
				}
			}
			if isStylesheet && href != "" {
				cssURL := resolveURL(base, href)
				localPath, err := downloadResource(cssURL, "css", base)
				if err == nil {
					b.WriteString("<link rel=\"stylesheet\" href=\"" + localPath + "\">")
				}
				return // skip writing the original <link>
			}
			if isPreload && href == "" {
				// Skip preload links without href to avoid browser errors
				return
			}
			if isPreload && href != "" {
				// Process preload links with href
				resourceURL := resolveURL(base, href)
				localPath, err := downloadResource(resourceURL, "css", base)
				if err == nil {
					// Write preload link with local path
					b.WriteString("<link rel=\"preload\" href=\"" + localPath + "\"")
					for _, attr := range n.Attr {
						if attr.Key != "rel" && attr.Key != "href" {
							b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
						}
					}
					b.WriteString(">")
				}
				return // skip writing the original <link>
			}
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			var src string
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					src = attr.Val
				}
			}
			if src != "" {
				scriptURL := resolveURL(base, src)
				localPath, err := downloadResource(scriptURL, "js", base)
				if err == nil {
					b.WriteString("<script src=\"" + localPath + "\"></script>")
				}
				return // skip writing the original <script src>
			} else {
				// Handle inline JavaScript - process the text content
				b.WriteString("<script")
				for _, attr := range n.Attr {
					b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
				}
				b.WriteString(">")
				
				// Process children (text nodes) for inline JavaScript
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.TextNode {
						processedJS, err := localizeJavaScriptURLs(c.Data, base)
						if err == nil {
							b.WriteString(processedJS)
						} else {
							b.WriteString(c.Data)
						}
					}
				}
				b.WriteString("</script>")
				return // skip normal processing for inline script tags
			}
		}
		if n.Type == html.ElementNode && n.Data == "img" {
			var src string
			var dataSrc string
			var srcset string
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					src = attr.Val
				}
				if attr.Key == "data-src" {
					dataSrc = attr.Val
				}
				if attr.Key == "srcset" {
					srcset = attr.Val
				}
			}

			// Process main src attribute
			if src != "" {
				imageURL := resolveURL(base, src)
				localPath, err := downloadImage(imageURL)
				if err == nil {
					src = localPath
				}
			}

			// Process data-src for lazy loading
			if dataSrc != "" {
				imageURL := resolveURL(base, dataSrc)
				localPath, err := downloadImage(imageURL)
				if err == nil {
					dataSrc = localPath
				}
			}

			// Process srcset attribute
			if srcset != "" {
				localizedSrcset, err := localizeSrcset(srcset, base)
				if err == nil {
					srcset = localizedSrcset
				}
			}

			// Write the img tag with updated paths
			b.WriteString("<img")
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					b.WriteString(" src=\"" + src + "\"")
				} else if attr.Key == "data-src" {
					b.WriteString(" data-src=\"" + dataSrc + "\"")
				} else if attr.Key == "srcset" {
					b.WriteString(" srcset=\"" + srcset + "\"")
				} else {
					b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
				}
			}
			b.WriteString(">")
			return // skip normal processing for img tags
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			var content string
			var property string
			var name string
			for _, attr := range n.Attr {
				if attr.Key == "content" {
					content = attr.Val
				}
				if attr.Key == "property" {
					property = attr.Val
				}
				if attr.Key == "name" {
					name = attr.Val
				}
			}

			// Process meta tags with image content
			isImageMeta := false
			if property == "og:image" || property == "og:image:secure_url" || name == "twitter:image" || name == "msapplication-TileImage" {
				isImageMeta = true
			}

			if isImageMeta && content != "" && (strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://")) {
				imageURL := resolveURL(base, content)
				localPath, err := downloadImage(imageURL)
				if err == nil {
					content = localPath
				}
			}

			// Write the meta tag with updated content
			b.WriteString("<meta")
			for _, attr := range n.Attr {
				if attr.Key == "content" {
					b.WriteString(" content=\"" + content + "\"")
				} else {
					b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
				}
			}
			b.WriteString(">")
			return // skip normal processing for meta tags
		}
		if n.Type == html.ElementNode {
			b.WriteString("<" + n.Data)
			for _, attr := range n.Attr {
				if (n.Data == "link" && attr.Key == "rel" && attr.Val == "stylesheet") || (n.Data == "link" && attr.Key == "href") || (n.Data == "script" && attr.Key == "src") || (n.Data == "img" && (attr.Key == "src" || attr.Key == "data-src" || attr.Key == "srcset")) || (n.Data == "meta" && attr.Key == "content") {
					continue // skip these attributes, handled above
				}
				
				// Process style attributes for background images
				if attr.Key == "style" && strings.Contains(attr.Val, "background-image") {
					updatedStyle, err := localizeStyleBackgroundImages(attr.Val, base)
					if err == nil {
						b.WriteString(" " + attr.Key + "=\"" + updatedStyle + "\"")
					} else {
						b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
					}
				} else {
					b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
				}
			}
			b.WriteString(">")
		}
		if n.Type == html.TextNode {
			// Check if this text node is inside a <style> tag
			if n.Parent != nil && n.Parent.Data == "style" {
				// Process CSS content for font URLs
				processedCSS, err := localizeFontURLs(n.Data, base)
				if err == nil {
					b.WriteString(processedCSS)
				} else {
					b.WriteString(n.Data)
				}
			} else {
				b.WriteString(n.Data)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
		if n.Type == html.ElementNode {
			b.WriteString("</" + n.Data + ">")
		}
	}
	f(doc)
	return b.String(), nil
}

func downloadResource(resourceURL, ext string, base *url.URL) (string, error) {
	resp, err := http.Get(resourceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(resourceURL)
	if err != nil {
		return "", err
	}
	segments := strings.Split(u.Path, "/")
	filename := segments[len(segments)-1]
	if !strings.HasSuffix(filename, "."+ext) {
		filename = filename + "." + ext
	}
	localPath := "assets/" + filename

	// If CSS, also localize font URLs and remove source maps
	if ext == "css" {
		cssContent := string(data)
		cssContent, err = localizeFontURLs(cssContent, base)
		if err != nil {
			return "", err
		}
		// Remove source map references
		cssContent = removeSourceMapReferences(cssContent)
		data = []byte(cssContent)
	}

	// If JS, remove source map references
	if ext == "js" {
		jsContent := string(data)
		jsContent = removeSourceMapReferences(jsContent)
		data = []byte(jsContent)
	}

	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	return localPath, nil
}

func addErrorSuppressionScript(htmlContent string) string {
	// Check if the script is already present
	if strings.Contains(htmlContent, "Suppress localhost development server connection errors") {
		return htmlContent
	}

	suppressionScript := `<script>
// Suppress localhost development server connection errors
window.addEventListener('error', function(e) {
    // Suppress errors related to localhost development servers and security errors
    if (e.message && (
        e.message.includes('localhost:127') || 
        e.message.includes('Failed to fetch') ||
        e.message.includes('NetworkError') ||
        e.message.includes('ERR_CONNECTION_REFUSED') ||
        e.message.includes('SecurityError') ||
        e.message.includes('Script origin does not match')
    )) {
        e.preventDefault();
        e.stopPropagation();
        return false;
    }
}, true);

// Suppress unhandled promise rejections for network errors
window.addEventListener('unhandledrejection', function(e) {
    if (e.reason && (
        e.reason.toString().includes('localhost:127') ||
        e.reason.toString().includes('Failed to fetch') ||
        e.reason.toString().includes('NetworkError') ||
        e.reason.toString().includes('ERR_CONNECTION_REFUSED') ||
        e.reason.toString().includes('SecurityError') ||
        e.reason.toString().includes('Script origin does not match') ||
        e.reason.toString().includes('registering client\'s origin')
    )) {
        e.preventDefault();
        return false;
    }
});

// Override console.error to filter localhost connection errors
if (!window.originalConsoleErrorOverridden) {
    window.originalConsoleErrorOverridden = true;
    const originalConsoleError = console.error;
    console.error = function(...args) {
        const message = args.join(' ');
        if (message.includes('localhost:127') || 
            message.includes('Failed to fetch') ||
            message.includes('ERR_CONNECTION_REFUSED') ||
            message.includes('SecurityError') ||
            message.includes('Script origin does not match')) {
            return; // Suppress these specific errors
        }
        originalConsoleError.apply(console, args);
    };
}
</script>`

	// Insert the script right after the opening <head> tag
	re := regexp.MustCompile(`(<head[^>]*>)`)
	return re.ReplaceAllString(htmlContent, "$1\n"+suppressionScript)
}

func removeSourceMapReferences(content string) string {
	// Remove both CSS and JS source map references
	// CSS: /*# sourceMappingURL=file.css.map */
	// JS: //# sourceMappingURL=file.js.map
	re := regexp.MustCompile(`(/\*#\s*sourceMappingURL=.*?\*/|//#\s*sourceMappingURL=.*?)`)
	return re.ReplaceAllString(content, "")
}

func localizeSrcset(srcsetContent string, base *url.URL) (string, error) {
	if srcsetContent == "" {
		return srcsetContent, nil
	}

	// Split srcset by comma to handle multiple entries
	entries := strings.Split(srcsetContent, ",")
	var localizedEntries []string

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Split entry into URL and descriptor (e.g., "image.jpg 2x" or "image.jpg 300w")
		parts := strings.Fields(entry)
		if len(parts) == 0 {
			continue
		}

		imageURL := parts[0]
		descriptor := ""
		if len(parts) > 1 {
			descriptor = " " + strings.Join(parts[1:], " ")
		}

		// Only process HTTP/HTTPS URLs
		if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
			resolvedURL := resolveURL(base, imageURL)
			localPath, err := downloadImage(resolvedURL)
			if err == nil {
				localizedEntries = append(localizedEntries, localPath+descriptor)
			} else {
				// If download failed, keep original URL
				localizedEntries = append(localizedEntries, entry)
			}
		} else {
			// Relative or other URL types - keep as is for now
			localizedEntries = append(localizedEntries, entry)
		}
	}

	return strings.Join(localizedEntries, ", "), nil
}

func downloadImage(imageURL string) (string, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(imageURL)
	if err != nil {
		return "", err
	}
	segments := strings.Split(u.Path, "/")
	filename := segments[len(segments)-1]

	// Handle images without extensions
	if !strings.Contains(filename, ".") {
		// Try to determine extension from content type
		contentType := resp.Header.Get("Content-Type")
		switch contentType {
		case "image/jpeg":
			filename += ".jpg"
		case "image/png":
			filename += ".png"
		case "image/gif":
			filename += ".gif"
		case "image/webp":
			filename += ".webp"
		case "image/svg+xml":
			filename += ".svg"
		default:
			filename += ".jpg" // default fallback
		}
	}

	localPath := "assets/images/" + filename

	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	return localPath, nil
}

func localizeStyleBackgroundImages(styleContent string, base *url.URL) (string, error) {
	// Regex to find background-image: url(...) in style attributes
	re := regexp.MustCompile(`background-image:\s*url\(['"]?([^'"]+)['"]?\)`)
	matches := re.FindAllStringSubmatch(styleContent, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		imagePath := match[1]
		
		// Only process if it's an HTTP/HTTPS URL
		if strings.HasPrefix(imagePath, "http://") || strings.HasPrefix(imagePath, "https://") {
			imageURL := resolveURL(base, imagePath)
			localPath, err := downloadImage(imageURL)
			if err == nil {
				// Replace the original URL with local path
				styleContent = strings.ReplaceAll(styleContent, imagePath, localPath)
			}
		}
	}
	return styleContent, nil
}

func localizeJavaScriptURLs(jsContent string, base *url.URL) (string, error) {
	// Handle template URLs with placeholders like {banner_id}, {type}
	// Account for escaped slashes in JavaScript
	templateRe := regexp.MustCompile(`"([^"]*\\?\/[^"]*\{[^}]+\}[^"]*\.(?:css|js)(?:\?[^"]*)?)"`)
	templateMatches := templateRe.FindAllStringSubmatch(jsContent, -1)
	
	for _, match := range templateMatches {
		if len(match) < 2 {
			continue
		}
		
		templateURL := match[1]
		// Unescape the URL for processing
		unescapedURL := strings.ReplaceAll(templateURL, "\\/", "/")
		
		// Extract placeholder variables like {banner_id}, {type}
		placeholderRe := regexp.MustCompile(`\{([^}]+)\}`)
		placeholders := placeholderRe.FindAllStringSubmatch(unescapedURL, -1)
		
		resolvedURL := unescapedURL
		
		// Try to resolve each placeholder by finding its value in the JavaScript
		for _, placeholder := range placeholders {
			if len(placeholder) < 2 {
				continue
			}
			
			placeholderName := placeholder[1]
			placeholderPattern := "{" + placeholderName + "}"
			
			// Look for the variable value in the JavaScript content
			var value string
			
			// Try different patterns to find the variable value
			patterns := []string{
				`"` + placeholderName + `":\s*"([^"]+)"`,           // "banner_id":"1"
				`"user_` + placeholderName + `":\s*"([^"]+)"`,      // "user_banner_id":"1"  
				`"` + placeholderName + `":\s*(\d+)`,               // "banner_id":1
				`"user_` + placeholderName + `":\s*(\d+)`,          // "user_banner_id":1
				`"consenttype":\s*"([^"]+)"`,                       // Special case for "type" -> "consenttype"
			}
			
			// Special mapping for common template variables
			if placeholderName == "type" {
				// For {type}, look for consenttype value
				re := regexp.MustCompile(`"consenttype":\s*"([^"]+)"`)
				if matches := re.FindStringSubmatch(jsContent); len(matches) > 1 {
					value = matches[1]
				}
			} else {
				for _, pattern := range patterns {
					re := regexp.MustCompile(pattern)
					if matches := re.FindStringSubmatch(jsContent); len(matches) > 1 {
						value = matches[1]
						break
					}
				}
			}
			
			// If we found a value, replace the placeholder
			if value != "" {
				resolvedURL = strings.ReplaceAll(resolvedURL, placeholderPattern, value)
			}
		}
		
		// If we successfully resolved all placeholders (no more { } remaining)
		if !strings.Contains(resolvedURL, "{") && !strings.Contains(resolvedURL, "}") {
			// Download the resolved CSS file
			if strings.Contains(resolvedURL, ".css") {
				cssURL := resolveURL(base, resolvedURL)
				localPath, err := downloadResource(cssURL, "css", base)
				if err == nil {
					// Replace the template URL with local path in JavaScript
					jsContent = strings.ReplaceAll(jsContent, `"`+templateURL+`"`, `"`+localPath+`"`)
				}
			}
		}
	}
	
	// General regex to find direct URLs in JavaScript strings (with escaped slashes)
	re := regexp.MustCompile(`"(https?:\\?\/\\?\/[^"]*\.(?:css|js|png|jpg|jpeg|gif|webp|svg)(?:\?[^"]*)?)"`)
	matches := re.FindAllStringSubmatch(jsContent, -1)
	
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		
		url := match[1]
		unescapedURL := strings.ReplaceAll(url, "\\/", "/")
		
		// Check if it's a CSS file we should download
		if strings.Contains(unescapedURL, ".css") {
			cssURL := resolveURL(base, unescapedURL)
			localPath, err := downloadResource(cssURL, "css", base)
			if err == nil {
				// Replace the URL with local path in the JavaScript
				jsContent = strings.ReplaceAll(jsContent, `"`+url+`"`, `"`+localPath+`"`)
			}
		}
	}
	
	return jsContent, nil
}

func localizeFontURLs(cssContent string, base *url.URL) (string, error) {
	fontDir := "assets/fonts/"
	os.MkdirAll(fontDir, 0755)
	// Regex to find url(...) - matches both HTTP URLs and relative paths
	re := regexp.MustCompile(`url\((['"]?)([^)'"]+)['"]?\)`)
	matches := re.FindAllStringSubmatch(cssContent, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		fontPath := match[2]

		// Convert relative paths to absolute URLs
		var fontURL string
		if strings.HasPrefix(fontPath, "http://") || strings.HasPrefix(fontPath, "https://") {
			// Already absolute URL
			fontURL = fontPath
		} else if strings.HasPrefix(fontPath, "//") {
			// Protocol-relative URL - use base URL's scheme
			fontURL = base.Scheme + ":" + fontPath
		} else {
			// Relative path - resolve against base URL
			fontURL = resolveURL(base, fontPath)
		}
		fontResp, err := http.Get(fontURL)
		if err != nil {
			continue
		}
		fontData, err := io.ReadAll(fontResp.Body)
		fontResp.Body.Close()
		if err != nil {
			continue
		}
		fontU, err := url.Parse(fontURL)
		if err != nil {
			continue
		}
		fontSegments := strings.Split(fontU.Path, "/")
		fontFilename := fontSegments[len(fontSegments)-1]
		localFontPath := fontDir + fontFilename
		os.WriteFile(localFontPath, fontData, 0644)
		// Replace both original path and resolved URL with local path in CSS
		relativeFontPath := "fonts/" + fontFilename
		cssContent = strings.ReplaceAll(cssContent, fontPath, relativeFontPath)
		if fontPath != fontURL {
			// Also replace the resolved URL in case it appears elsewhere
			cssContent = strings.ReplaceAll(cssContent, fontURL, relativeFontPath)
		}
	}
	return cssContent, nil
}

func resolveURL(base *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}
