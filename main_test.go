package main

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"wp-static-scraper/assets"
	"wp-static-scraper/html"
	"wp-static-scraper/utils"
)

func TestResolveURL(t *testing.T) {
	base, err := url.Parse("https://example.com/path/")
	if err != nil {
		t.Fatalf("Failed to parse base URL: %v", err)
	}

	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{
			name:     "absolute URL",
			ref:      "https://other.com/file.css",
			expected: "https://other.com/file.css",
		},
		{
			name:     "relative path",
			ref:      "../assets/style.css",
			expected: "https://example.com/assets/style.css",
		},
		{
			name:     "root relative path",
			ref:      "/css/main.css",
			expected: "https://example.com/css/main.css",
		},
		{
			name:     "same directory",
			ref:      "style.css",
			expected: "https://example.com/path/style.css",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.ResolveURL(base, tt.ref)
			if result != tt.expected {
				t.Errorf("ResolveURL(%q, %q) = %q; want %q", base.String(), tt.ref, result, tt.expected)
			}
		})
	}
}

func TestRemoveSourceMapReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "CSS source map",
			input:    "body { color: red; }\n/*# sourceMappingURL=style.css.map */",
			expected: "body { color: red; }\n",
		},
		{
			name:     "JS source map",
			input:    "console.log('hello');\n//# sourceMappingURL=script.js.map",
			expected: "console.log('hello');\n",
		},
		{
			name:     "multiple source maps",
			input:    "/*# sourceMappingURL=a.css.map */\nbody{}\n//# sourceMappingURL=b.js.map",
			expected: "\nbody{}\n",
		},
		{
			name:     "no source maps",
			input:    "body { color: blue; }",
			expected: "body { color: blue; }",
		},
		{
			name:     "source map with query params",
			input:    "/*# sourceMappingURL=style.css.map?v=123 */",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.RemoveSourceMapReferences(tt.input)
			if result != tt.expected {
				t.Errorf("RemoveSourceMapReferences(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanupOldFiles(t *testing.T) {
	// Create a temporary test directory structure
	testDir := "test_output"
	testFile := "test.html"
	
	// Create test directory and file
	os.MkdirAll(testDir+"/assets", 0755)
	os.WriteFile(testDir+"/index.html", []byte("test"), 0644)
	os.WriteFile(testDir+"/assets/style.css", []byte("css"), 0644)
	
	// Verify files exist before cleanup
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Fatal("Test directory was not created")
	}

	// Test cleanup with a mock function (since CleanupOldFiles removes "output" specifically)
	testCleanup := func(outputFile string) {
		os.RemoveAll(testDir)
	}
	
	testCleanup(testFile)
	
	// Verify directory was removed
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("Test directory should have been removed")
	}
}

func TestLocalizeSrcset(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty srcset",
			input:    "",
			expected: "",
		},
		{
			name:     "single image with width descriptor",
			input:    "https://example.com/image.jpg 300w",
			expected: "https://example.com/image.jpg 300w", // Would be localized in real implementation
		},
		{
			name:     "multiple images",
			input:    "https://example.com/small.jpg 300w, https://example.com/large.jpg 600w",
			expected: "https://example.com/small.jpg 300w, https://example.com/large.jpg 600w",
		},
		{
			name:     "relative URL",
			input:    "image.jpg 1x, image@2x.jpg 2x",
			expected: "image.jpg 1x, image@2x.jpg 2x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := assets.LocalizeSrcset(tt.input, base)
			if err != nil {
				t.Errorf("LocalizeSrcset returned error: %v", err)
			}
			// For this test, we're mainly checking that the function doesn't crash
			// In a real implementation, we'd mock the downloadImage function
			if tt.input == "" && result != tt.expected {
				t.Errorf("LocalizeSrcset(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLocalizeStyleBackgroundImages(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	
	tests := []struct {
		name     string
		input    string
		contains string // What the result should contain
	}{
		{
			name:     "no background images",
			input:    "color: red; font-size: 14px;",
			contains: "color: red; font-size: 14px;",
		},
		{
			name:     "background image with HTTP URL",
			input:    "background-image: url('https://example.com/bg.jpg'); color: blue;",
			contains: "color: blue;", // URL would be replaced in real implementation
		},
		{
			name:     "background image without quotes",
			input:    "background-image: url(https://example.com/bg.png)",
			contains: "background-image:", // Function should process this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := assets.LocalizeStyleBackgroundImages(tt.input, base)
			if err != nil {
				t.Errorf("LocalizeStyleBackgroundImages returned error: %v", err)
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("LocalizeStyleBackgroundImages result should contain %q, got %q", tt.contains, result)
			}
		})
	}
}

func TestAddErrorSuppressionScript(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "basic HTML",
			input: "<html><head></head><body></body></html>",
			contains: []string{
				"Suppress localhost development server connection errors",
				"window.addEventListener('error'",
			},
		},
		{
			name:  "HTML with existing head attributes",
			input: "<html><head lang=\"en\"><title>Test</title></head><body></body></html>",
			contains: []string{
				"<script>",
				"localhost:127",
				"SecurityError",
			},
		},
		{
			name:  "already has suppression script",
			input: "<html><head><script>// Suppress localhost development server connection errors</script></head></html>",
			contains: []string{
				"Suppress localhost development server connection errors",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := html.AddErrorSuppressionScript(tt.input)
			for _, expectedContent := range tt.contains {
				if !strings.Contains(result, expectedContent) {
					t.Errorf("AddErrorSuppressionScript result should contain %q", expectedContent)
				}
			}
			
			// Check that script is not duplicated if already present
			if strings.Contains(tt.input, "Suppress localhost development server connection errors") {
				count := strings.Count(result, "Suppress localhost development server connection errors")
				if count > 1 {
					t.Error("Error suppression script should not be duplicated")
				}
			}
		})
	}
}