package commands

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

// ServeCommand starts an HTTP server to serve scraped content
func ServeCommand() {
	serveFlags := flag.NewFlagSet("serve", flag.ExitOnError)
	port := serveFlags.Int("port", 8080, "Port for HTTP server")
	serveFlags.Parse(os.Args[2:])

	// Check if output directory and index.html exists
	if _, err := os.Stat("output/index.html"); os.IsNotExist(err) {
		fmt.Println("output/index.html not found. Please run 'scrape' command first.")
		os.Exit(1)
	}

	// Set up file server for static assets
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("output/assets"))))

	// Handle direct /webfonts/ requests (for CSS files that reference absolute webfonts paths)
	http.Handle("/webfonts/", http.StripPrefix("/webfonts/", http.FileServer(http.Dir("output/assets/fonts"))))

	// Handle direct /fonts/ requests (for CSS files that reference fonts/ paths)
	http.Handle("/fonts/", http.StripPrefix("/fonts/", http.FileServer(http.Dir("output/assets/fonts"))))

	// Handle direct /images/ requests for downloaded images
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("output/assets/images"))))

	// Serve index.html at root
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "output/index.html")
		} else {
			http.NotFound(w, r)
		}
	})

	fmt.Printf("Starting server on http://localhost:%d\n", *port)
	fmt.Println("Press Ctrl+C to stop the server")
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}