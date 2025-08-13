package commands

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"wp-static-scraper/assets"
	"wp-static-scraper/html"
	"wp-static-scraper/utils"
)

// ScrapeCommand handles the scraping workflow
func ScrapeCommand() {
	startTime := time.Now()
	
	scrapeFlags := flag.NewFlagSet("scrape", flag.ExitOnError)
	inputURL := scrapeFlags.String("url", "", "URL of the website to scrape")
	outputFile := scrapeFlags.String("out", "index.html", "Output HTML file")
	concurrency := scrapeFlags.Int("concurrency", 10, "Number of concurrent downloads (1-50)")
	scrapeFlags.Parse(os.Args[2:])

	if *inputURL == "" {
		fmt.Println("Please provide a URL with -url flag.")
		scrapeFlags.Usage()
		os.Exit(1)
	}

	// Validate concurrency parameter
	if *concurrency < 1 || *concurrency > 50 {
		fmt.Println("Concurrency must be between 1 and 50.")
		os.Exit(1)
	}

	// Clean up old files before starting new scrape
	utils.CleanupOldFiles(*outputFile)

	// Ensure output directories exist
	if err := utils.EnsureDirectories(); err != nil {
		fmt.Printf("Failed to create directories: %v\n", err)
		os.Exit(1)
	}

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

	updatedHTML, err := assets.LocalizeAssets(string(body), base, *concurrency)
	if err != nil {
		fmt.Printf("Failed to localize assets: %v\n", err)
		os.Exit(1)
	}

	// Add script to suppress localhost development server errors
	updatedHTML = html.AddErrorSuppressionScript(updatedHTML)

	err = os.WriteFile("output/"+*outputFile, []byte(updatedHTML), 0644)
	if err != nil {
		fmt.Printf("Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	totalTime := time.Since(startTime)
	fmt.Printf("Static HTML with local assets saved to output/%s\n", *outputFile)
	fmt.Printf("Total execution time: %.2fs\n", totalTime.Seconds())
}