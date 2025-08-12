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
	// Remove assets directory and all its contents
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

	updatedHTML, err := localizeAssets(string(body), base)
	if err != nil {
		fmt.Printf("Failed to localize assets: %v\n", err)
		os.Exit(1)
	}

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
			var href string
			for _, attr := range n.Attr {
				if attr.Key == "rel" && attr.Val == "stylesheet" {
					isStylesheet = true
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
			}
		}
		if n.Type == html.ElementNode {
			b.WriteString("<" + n.Data)
			for _, attr := range n.Attr {
				if (n.Data == "link" && attr.Key == "rel" && attr.Val == "stylesheet") || (n.Data == "link" && attr.Key == "href") || (n.Data == "script" && attr.Key == "src") {
					continue // skip these attributes, handled above
				}
				b.WriteString(" " + attr.Key + "=\"" + attr.Val + "\"")
			}
			b.WriteString(">")
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
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

	// If CSS, also localize font URLs
	if ext == "css" {
		cssContent := string(data)
		cssContent, err = localizeFontURLs(cssContent, base)
		if err != nil {
			return "", err
		}
		data = []byte(cssContent)
	}

	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	return localPath, nil
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
