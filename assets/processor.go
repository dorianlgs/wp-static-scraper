package assets

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"wp-static-scraper/utils"
)

// LocalizeAssets processes HTML content and localizes all assets using concurrent downloads
func LocalizeAssets(htmlContent string, base *url.URL, concurrency int) (string, error) {
	fmt.Printf("Starting asset localization with %d concurrent workers...\n", concurrency)
	
	// Phase 1: Collect all asset URLs without downloading
	assetJobs, err := collectAssetJobs(htmlContent, base)
	if err != nil {
		return "", err
	}
	
	if len(assetJobs) == 0 {
		fmt.Println("No assets found to download.")
		return htmlContent, nil
	}
	
	fmt.Printf("Found %d assets to download\n", len(assetJobs))
	
	// Phase 2: Download primary assets (CSS, JS, Images) in parallel
	downloader := NewConcurrentDownloader(concurrency)
	downloader.Start()
	
	// Start progress reporting
	reporter := NewProgressReporter(downloader, 1*time.Second)
	reporter.Start()
	
	// Queue all primary asset jobs
	for _, job := range assetJobs {
		downloader.AddJob(job)
	}
	downloader.FinishJobs()
	
	// Get results from primary downloads
	urlMap := downloader.GetResults()
	reporter.Stop()
	
	fmt.Printf("Downloaded %d assets successfully\n", len(urlMap))
	
	// Phase 3: Process CSS files for fonts and download fonts in parallel
	fontJobs, err := collectFontJobs(urlMap, base)
	if err != nil {
		return "", err
	}
	
	if len(fontJobs) > 0 {
		fmt.Printf("Found %d fonts to download\n", len(fontJobs))
		
		// Download fonts in parallel
		fontDownloader := NewConcurrentDownloader(concurrency)
		fontDownloader.Start()
		
		fontReporter := NewProgressReporter(fontDownloader, 1*time.Second)
		fontReporter.Start()
		
		for _, job := range fontJobs {
			fontDownloader.AddJob(job)
		}
		fontDownloader.FinishJobs()
		
		fontMap := fontDownloader.GetResults()
		fontReporter.Stop()
		
		// Merge font results into main URL map
		for k, v := range fontMap {
			urlMap[k] = v
		}
		
		fmt.Printf("Downloaded %d fonts successfully\n", len(fontMap))
	}
	
	// Phase 4: Update HTML with all localized asset references
	updatedHTML, err := updateHTMLWithLocalPaths(htmlContent, base, urlMap)
	if err != nil {
		return "", err
	}
	
	return updatedHTML, nil
}

// collectAssetJobs parses HTML and collects all asset download jobs
func collectAssetJobs(htmlContent string, base *url.URL) ([]DownloadJob, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}
	
	var jobs []DownloadJob
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		// Collect CSS and JS from <link> and <script> tags
		if n.Type == html.ElementNode && n.Data == "link" {
			var href, rel string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
				}
				if attr.Key == "rel" {
					rel = attr.Val
				}
			}
			if (rel == "stylesheet" || rel == "preload") && href != "" {
				resolvedURL := utils.ResolveURL(base, href)
				jobs = append(jobs, DownloadJob{
					URL:          resolvedURL,
					Type:         "css",
					OriginalPath: href,
					BaseURL:      base,
				})
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
				resolvedURL := utils.ResolveURL(base, src)
				jobs = append(jobs, DownloadJob{
					URL:          resolvedURL,
					Type:         "js",
					OriginalPath: src,
					BaseURL:      base,
				})
			}
		}
		
		// Collect images from <img> tags
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, attr := range n.Attr {
				var src string
				if attr.Key == "src" || attr.Key == "data-src" {
					src = attr.Val
				}
				if src != "" && (strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")) {
					resolvedURL := utils.ResolveURL(base, src)
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "image",
						OriginalPath: src,
						BaseURL:      base,
					})
				}
				// Handle srcset
				if attr.Key == "srcset" {
					srcsetJobs := collectSrcsetJobs(attr.Val, base)
					jobs = append(jobs, srcsetJobs...)
				}
			}
		}
		
		// Collect images from <meta> tags
		if n.Type == html.ElementNode && n.Data == "meta" {
			var content, property, name string
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
			
			isImageMeta := property == "og:image" || property == "og:image:secure_url" || 
				name == "twitter:image" || name == "msapplication-TileImage"
			
			if isImageMeta && content != "" && (strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://")) {
				resolvedURL := utils.ResolveURL(base, content)
				jobs = append(jobs, DownloadJob{
					URL:          resolvedURL,
					Type:         "image",
					OriginalPath: content,
					BaseURL:      base,
				})
			}
		}
		
		// Collect background images from style attributes
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "style" && strings.Contains(attr.Val, "background-image") {
					styleJobs := collectStyleBackgroundJobs(attr.Val, base)
					jobs = append(jobs, styleJobs...)
				}
			}
		}
		
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	
	traverse(doc)
	return jobs, nil
}

// collectSrcsetJobs extracts image URLs from srcset attributes
func collectSrcsetJobs(srcsetContent string, base *url.URL) []DownloadJob {
	var jobs []DownloadJob
	
	entries := strings.Split(srcsetContent, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		
		parts := strings.Fields(entry)
		if len(parts) == 0 {
			continue
		}
		
		imageURL := parts[0]
		if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
			resolvedURL := utils.ResolveURL(base, imageURL)
			jobs = append(jobs, DownloadJob{
				URL:          resolvedURL,
				Type:         "image",
				OriginalPath: imageURL,
				BaseURL:      base,
			})
		}
	}
	
	return jobs
}

// collectStyleBackgroundJobs extracts background image URLs from style attributes
func collectStyleBackgroundJobs(styleContent string, base *url.URL) []DownloadJob {
	var jobs []DownloadJob
	
	re := regexp.MustCompile(`background-image:\s*url\(['"]?([^'"]+)['"]?\)`)
	matches := re.FindAllStringSubmatch(styleContent, -1)
	
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		imagePath := match[1]
		
		if strings.HasPrefix(imagePath, "http://") || strings.HasPrefix(imagePath, "https://") {
			resolvedURL := utils.ResolveURL(base, imagePath)
			jobs = append(jobs, DownloadJob{
				URL:          resolvedURL,
				Type:         "image",
				OriginalPath: imagePath,
				BaseURL:      base,
			})
		}
	}
	
	return jobs
}

// collectFontJobs processes downloaded CSS files to find font URLs
func collectFontJobs(urlMap map[string]string, base *url.URL) ([]DownloadJob, error) {
	var jobs []DownloadJob
	
	for _, localPath := range urlMap {
		// Only process CSS files
		if !strings.Contains(localPath, ".css") {
			continue
		}
		
		// Read the downloaded CSS file
		cssContent, err := os.ReadFile(localPath)
		if err != nil {
			continue // Skip if can't read
		}
		
		// Find font URLs in CSS content
		fontJobs := collectFontJobsFromCSS(string(cssContent), base)
		jobs = append(jobs, fontJobs...)
	}
	
	return jobs, nil
}

// collectFontJobsFromCSS extracts font URLs from CSS content
func collectFontJobsFromCSS(cssContent string, base *url.URL) []DownloadJob {
	var jobs []DownloadJob
	
	re := regexp.MustCompile(`url\((['"]?)([^)'"]+)['"]?\)`)
	matches := re.FindAllStringSubmatch(cssContent, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		fontPath := match[2]
		
		// Check if it's a font file
		isFontFile := strings.HasSuffix(fontPath, ".woff") || 
			strings.HasSuffix(fontPath, ".woff2") ||
			strings.HasSuffix(fontPath, ".ttf") ||
			strings.HasSuffix(fontPath, ".eot") ||
			strings.HasSuffix(fontPath, ".svg")
		
		if !isFontFile {
			continue
		}
		
		// Convert relative paths to absolute URLs
		var fontURL string
		if strings.HasPrefix(fontPath, "http://") || strings.HasPrefix(fontPath, "https://") {
			fontURL = fontPath
		} else if strings.HasPrefix(fontPath, "//") {
			fontURL = base.Scheme + ":" + fontPath
		} else {
			fontURL = utils.ResolveURL(base, fontPath)
		}
		
		jobs = append(jobs, DownloadJob{
			URL:          fontURL,
			Type:         "font",
			OriginalPath: fontPath,
			BaseURL:      base,
		})
	}
	
	return jobs
}

// updateHTMLWithLocalPaths updates HTML content with localized asset paths
func updateHTMLWithLocalPaths(htmlContent string, base *url.URL, urlMap map[string]string) (string, error) {
	// For now, use a simple string replacement approach
	// This could be optimized to use HTML parsing if needed
	updatedHTML := htmlContent
	
	for originalPath, localPath := range urlMap {
		// Convert output/assets/file.ext to assets/file.ext for HTML references
		relativePath := strings.TrimPrefix(localPath, "output/")
		updatedHTML = strings.ReplaceAll(updatedHTML, originalPath, relativePath)
	}
	
	return updatedHTML, nil
}

// LocalizeSrcset processes srcset attributes for responsive images
func LocalizeSrcset(srcsetContent string, base *url.URL) (string, error) {
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
			resolvedURL := utils.ResolveURL(base, imageURL)
			localPath, err := DownloadImage(resolvedURL)
			if err == nil {
				// Convert output/assets/images/file.jpg to assets/images/file.jpg for HTML references
				relativePath := strings.TrimPrefix(localPath, "output/")
				localizedEntries = append(localizedEntries, relativePath+descriptor)
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

// LocalizeStyleBackgroundImages processes background images in style attributes
func LocalizeStyleBackgroundImages(styleContent string, base *url.URL) (string, error) {
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
			imageURL := utils.ResolveURL(base, imagePath)
			localPath, err := DownloadImage(imageURL)
			if err == nil {
				// Convert output/assets/images/file.jpg to assets/images/file.jpg for HTML references
				relativePath := strings.TrimPrefix(localPath, "output/")
				// Replace the original URL with local path
				styleContent = strings.ReplaceAll(styleContent, imagePath, relativePath)
			}
		}
	}
	return styleContent, nil
}

// LocalizeJavaScriptURLs processes JavaScript content for embedded resource URLs
func LocalizeJavaScriptURLs(jsContent string, base *url.URL) (string, error) {
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
				cssURL := utils.ResolveURL(base, resolvedURL)
				localPath, err := DownloadResource(cssURL, "css", base)
				if err == nil {
					// Convert output/assets/file.css to assets/file.css for HTML references
					relativePath := strings.TrimPrefix(localPath, "output/")
					// Replace the template URL with local path in JavaScript
					jsContent = strings.ReplaceAll(jsContent, `"`+templateURL+`"`, `"`+relativePath+`"`)
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
			cssURL := utils.ResolveURL(base, unescapedURL)
			localPath, err := DownloadResource(cssURL, "css", base)
			if err == nil {
				// Convert output/assets/file.css to assets/file.css for HTML references
				relativePath := strings.TrimPrefix(localPath, "output/")
				// Replace the URL with local path in the JavaScript
				jsContent = strings.ReplaceAll(jsContent, `"`+url+`"`, `"`+relativePath+`"`)
			}
		}
	}
	
	return jsContent, nil
}

// LocalizeFontURLs processes CSS content for font URLs and downloads fonts
func LocalizeFontURLs(cssContent string, base *url.URL) (string, error) {
	fontDir := "output/assets/fonts/"
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
			fontURL = utils.ResolveURL(base, fontPath)
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