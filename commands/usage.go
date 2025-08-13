package commands

import "fmt"

// PrintUsage displays help information for available commands
func PrintUsage() {
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
	fmt.Println("  -url         URL of the website to scrape (required)")
	fmt.Println("  -out         Output HTML file (default: index.html)")
	fmt.Println("  -concurrency Number of concurrent downloads (default: 10, range: 1-50)")
	fmt.Println("")
	fmt.Println("Serve options:")
	fmt.Println("  -port     Port for HTTP server (default: 8080)")
}