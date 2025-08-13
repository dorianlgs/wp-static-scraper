package assets

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"wp-static-scraper/utils"
)

// DownloadResource downloads a resource (CSS, JS) and saves it locally
func DownloadResource(resourceURL, ext string, base *url.URL) (string, error) {
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
	localPath := "output/assets/" + filename

	// If CSS, also localize font URLs and remove source maps
	if ext == "css" {
		cssContent := string(data)
		cssContent, err = LocalizeFontURLs(cssContent, base)
		if err != nil {
			return "", err
		}
		// Remove source map references
		cssContent = utils.RemoveSourceMapReferences(cssContent)
		data = []byte(cssContent)
	}

	// If JS, remove source map references
	if ext == "js" {
		jsContent := string(data)
		jsContent = utils.RemoveSourceMapReferences(jsContent)
		data = []byte(jsContent)
	}

	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	return localPath, nil
}

// DownloadImage downloads an image and saves it locally
func DownloadImage(imageURL string) (string, error) {
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

	localPath := "output/assets/images/" + filename

	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	return localPath, nil
}