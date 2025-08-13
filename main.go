package main

import (
	"fmt"
	"os"

	"wp-static-scraper/commands"
)

func main() {
	if len(os.Args) < 2 {
		commands.PrintUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "scrape":
		commands.ScrapeCommand()
	case "serve":
		commands.ServeCommand()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		commands.PrintUsage()
		os.Exit(1)
	}
}