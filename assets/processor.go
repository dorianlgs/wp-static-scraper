package assets

import (
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
	// Phase 1: Collect ALL asset URLs including fonts from inline CSS upfront
	allJobs, err := collectAllAssetJobs(htmlContent, base)
	if err != nil {
		return "", err
	}
	
	if len(allJobs) == 0 {
		return htmlContent, nil
	}
	
	// Phase 2: Download ALL assets (CSS, JS, Images, Fonts) in parallel
	downloader := NewConcurrentDownloader(concurrency)
	downloader.Start()
	
	// Start progress reporting (reduced frequency for better performance)
	reporter := NewProgressReporter(downloader, 2*time.Second)
	reporter.Start()
	
	// Queue all asset jobs at once - no waiting for CSS to finish
	for _, job := range allJobs {
		downloader.AddJob(job)
	}
	downloader.FinishJobs()
	
	// Get results from all downloads
	urlMap := downloader.GetResults()
	reporter.Stop()
	
	// Phase 3: Process inline JavaScript for template URLs (like Complianz)
	htmlContent, err = processInlineJavaScript(htmlContent, base)
	if err != nil {
		return "", err
	}
	
	// Phase 4: Update HTML with all localized asset references
	updatedHTML, err := updateHTMLWithLocalPaths(htmlContent, base, urlMap)
	if err != nil {
		return "", err
	}
	
	return updatedHTML, nil
}

// collectAllAssetJobs parses HTML and collects ALL asset download jobs including fonts from inline CSS
func collectAllAssetJobs(htmlContent string, base *url.URL) ([]DownloadJob, error) {
	// First collect primary assets
	jobs, err := collectAssetJobs(htmlContent, base)
	if err != nil {
		return nil, err
	}
	
	// Then collect fonts from inline CSS in <style> tags
	fontJobs := collectInlineFontJobs(htmlContent, base)
	jobs = append(jobs, fontJobs...)
	
	return jobs, nil
}

// collectAssetJobs parses HTML and collects primary asset download jobs
func collectAssetJobs(htmlContent string, base *url.URL) ([]DownloadJob, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}
	
	var jobs []DownloadJob
	urlSeen := make(map[string]bool) // Prevent duplicates
	
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
				if !urlSeen[resolvedURL] {
					urlSeen[resolvedURL] = true
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "css",
						OriginalPath: href,
						BaseURL:      base,
					})
				}
			}
			if rel == "manifest" && href != "" {
				resolvedURL := utils.ResolveURL(base, href)
				if !urlSeen[resolvedURL] {
					urlSeen[resolvedURL] = true
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "json",
						OriginalPath: href,
						BaseURL:      base,
					})
				}
			}
			if (rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon") && href != "" {
				resolvedURL := utils.ResolveURL(base, href)
				if !urlSeen[resolvedURL] {
					urlSeen[resolvedURL] = true
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "image",
						OriginalPath: href,
						BaseURL:      base,
					})
				}
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
				if !urlSeen[resolvedURL] {
					urlSeen[resolvedURL] = true
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "js",
						OriginalPath: src,
						BaseURL:      base,
					})
				}
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
					if !urlSeen[resolvedURL] {
						urlSeen[resolvedURL] = true
						jobs = append(jobs, DownloadJob{
							URL:          resolvedURL,
							Type:         "image",
							OriginalPath: src,
							BaseURL:      base,
						})
					}
				}
				// Handle srcset
				if attr.Key == "srcset" {
					srcsetJobs := collectSrcsetJobsWithDupeCheck(attr.Val, base, urlSeen)
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
				if !urlSeen[resolvedURL] {
					urlSeen[resolvedURL] = true
					jobs = append(jobs, DownloadJob{
						URL:          resolvedURL,
						Type:         "image",
						OriginalPath: content,
						BaseURL:      base,
					})
				}
			}
		}
		
		// Collect background images from style attributes
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "style" && strings.Contains(attr.Val, "background-image") {
					styleJobs := collectStyleBackgroundJobsWithDupeCheck(attr.Val, base, urlSeen)
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

// collectSrcsetJobs extracts image URLs from srcset attributes (legacy function)
func collectSrcsetJobs(srcsetContent string, base *url.URL) []DownloadJob {
	urlSeen := make(map[string]bool)
	return collectSrcsetJobsWithDupeCheck(srcsetContent, base, urlSeen)
}

// collectSrcsetJobsWithDupeCheck extracts image URLs from srcset attributes with duplicate checking
func collectSrcsetJobsWithDupeCheck(srcsetContent string, base *url.URL, urlSeen map[string]bool) []DownloadJob {
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
			if !urlSeen[resolvedURL] {
				urlSeen[resolvedURL] = true
				jobs = append(jobs, DownloadJob{
					URL:          resolvedURL,
					Type:         "image",
					OriginalPath: imageURL,
					BaseURL:      base,
				})
			}
		}
	}
	
	return jobs
}

// collectStyleBackgroundJobs extracts background image URLs from style attributes (legacy function)
func collectStyleBackgroundJobs(styleContent string, base *url.URL) []DownloadJob {
	urlSeen := make(map[string]bool)
	return collectStyleBackgroundJobsWithDupeCheck(styleContent, base, urlSeen)
}

// collectStyleBackgroundJobsWithDupeCheck extracts background image URLs from style attributes with duplicate checking
func collectStyleBackgroundJobsWithDupeCheck(styleContent string, base *url.URL, urlSeen map[string]bool) []DownloadJob {
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
			if !urlSeen[resolvedURL] {
				urlSeen[resolvedURL] = true
				jobs = append(jobs, DownloadJob{
					URL:          resolvedURL,
					Type:         "image",
					OriginalPath: imagePath,
					BaseURL:      base,
				})
			}
		}
	}
	
	return jobs
}


// collectInlineFontJobs extracts font URLs from inline CSS within <style> tags
func collectInlineFontJobs(htmlContent string, base *url.URL) []DownloadJob {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}
	
	var jobs []DownloadJob
	urlSeen := make(map[string]bool)
	
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "style" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				cssContent := n.FirstChild.Data
				fontJobs := collectFontJobsFromCSS(cssContent, base)
				
				// Add to jobs with duplicate checking
				for _, job := range fontJobs {
					if !urlSeen[job.URL] {
						urlSeen[job.URL] = true
						jobs = append(jobs, job)
					}
				}
			}
		}
		
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	
	traverse(doc)
	return jobs
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

// processInlineJavaScript processes inline script tags for template URLs
func processInlineJavaScript(htmlContent string, base *url.URL) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}
	
	var processScript func(*html.Node)
	processScript = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			// Check if this is an inline script (no src attribute)
			var hasSrc bool
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					hasSrc = true
					break
				}
			}
			
			if !hasSrc && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				// Process inline JavaScript content
				scriptContent := n.FirstChild.Data
				processedContent, err := LocalizeJavaScriptURLs(scriptContent, base)
				if err == nil && processedContent != scriptContent {
					n.FirstChild.Data = processedContent
				}
			}
		}
		
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			processScript(c)
		}
	}
	
	processScript(doc)
	
	// Convert back to HTML
	var buf strings.Builder
	err = html.Render(&buf, doc)
	if err != nil {
		return "", err
	}
	
	return buf.String(), nil
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
	// Account for escaped slashes in JavaScript - handle both \/ and / patterns
	templateRe := regexp.MustCompile(`"([^"]*\\?\/[^"]*\{[^}]+\}[^"]*\.(?:css|js)(?:\?[^"]*)?)"`)
	templateMatches := templateRe.FindAllStringSubmatch(jsContent, -1)
	
	// Also try a more flexible pattern for JSON-encoded URLs (css_file field)
	if len(templateMatches) == 0 {
		// Pattern specifically for "css_file":"https:\/\/..." format
		cssFileRe := regexp.MustCompile(`"css_file":"([^"]*\\?\/[^"]*\{[^}]+\}[^"]*\.(?:css|js)(?:\?[^"]*)?)"`)
		templateMatches = cssFileRe.FindAllStringSubmatch(jsContent, -1)
	}
	
	// Special handling for Complianz banner CSS with complete template resolution
	if strings.Contains(jsContent, "css_file") && strings.Contains(jsContent, "banner-{banner_id}-{type}") {
		// Extract user_banner_id and consenttype from the JSON object
		userBannerIdRe := regexp.MustCompile(`"user_banner_id":"([^"]+)"`)
		consentTypeRe := regexp.MustCompile(`"consenttype":"([^"]+)"`)
		cssFileRe := regexp.MustCompile(`"css_file":"([^"]*banner-\{banner_id\}-\{type\}[^"]*)"`)
		
		userBannerMatch := userBannerIdRe.FindStringSubmatch(jsContent)
		consentTypeMatch := consentTypeRe.FindStringSubmatch(jsContent)
		cssFileMatch := cssFileRe.FindStringSubmatch(jsContent)
		
		if len(userBannerMatch) > 1 && len(consentTypeMatch) > 1 && len(cssFileMatch) > 1 {
			bannerId := userBannerMatch[1]
			consentType := consentTypeMatch[1]
			templateURL := cssFileMatch[1]
			
			// Resolve the template URL
			resolvedURL := strings.ReplaceAll(templateURL, "{banner_id}", bannerId)
			resolvedURL = strings.ReplaceAll(resolvedURL, "{type}", consentType)
			resolvedURL = strings.ReplaceAll(resolvedURL, `\/`, "/") // Unescape JSON slashes
			
			// Download the resolved CSS file
			localPath, err := DownloadResource(resolvedURL, "css", base)
			if err == nil {
				relativePath := strings.TrimPrefix(localPath, "output/")
				// Replace both the template URL and resolved URL with local path
				jsContent = strings.ReplaceAll(jsContent, templateURL, relativePath)
				jsContent = strings.ReplaceAll(jsContent, resolvedURL, relativePath)
			}
		}
	}
	
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