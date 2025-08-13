package utils

import (
	"net/url"
	"regexp"
)

// ResolveURL resolves a relative URL against a base URL
func ResolveURL(base *url.URL, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}

// RemoveSourceMapReferences removes source map references from CSS and JS content
func RemoveSourceMapReferences(content string) string {
	// Remove both CSS and JS source map references
	// CSS: /*# sourceMappingURL=file.css.map */
	// JS: //# sourceMappingURL=file.js.map
	re := regexp.MustCompile(`(/\*#\s*sourceMappingURL=.*?\*/|//#\s*sourceMappingURL=.*)`)
	return re.ReplaceAllString(content, "")
}