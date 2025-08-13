package html

import (
	"regexp"
	"strings"
)

// AddErrorSuppressionScript adds JavaScript to suppress localhost development server errors
func AddErrorSuppressionScript(htmlContent string) string {
	// Check if the script is already present
	if strings.Contains(htmlContent, "Suppress localhost development server connection errors") {
		return htmlContent
	}

	suppressionScript := `<script>
// Suppress localhost development server connection errors
window.addEventListener('error', function(e) {
    // Suppress errors related to localhost development servers and security errors
    if (e.message && (
        e.message.includes('localhost:127') || 
        e.message.includes('Failed to fetch') ||
        e.message.includes('NetworkError') ||
        e.message.includes('ERR_CONNECTION_REFUSED') ||
        e.message.includes('SecurityError') ||
        e.message.includes('Script origin does not match')
    )) {
        e.preventDefault();
        e.stopPropagation();
        return false;
    }
}, true);

// Suppress unhandled promise rejections for network errors
window.addEventListener('unhandledrejection', function(e) {
    if (e.reason && (
        e.reason.toString().includes('localhost:127') ||
        e.reason.toString().includes('Failed to fetch') ||
        e.reason.toString().includes('NetworkError') ||
        e.reason.toString().includes('ERR_CONNECTION_REFUSED') ||
        e.reason.toString().includes('SecurityError') ||
        e.reason.toString().includes('Script origin does not match') ||
        e.reason.toString().includes('registering client\'s origin')
    )) {
        e.preventDefault();
        return false;
    }
});

// Override console.error to filter localhost connection errors
if (!window.originalConsoleErrorOverridden) {
    window.originalConsoleErrorOverridden = true;
    const originalConsoleError = console.error;
    console.error = function(...args) {
        const message = args.join(' ');
        if (message.includes('localhost:127') || 
            message.includes('Failed to fetch') ||
            message.includes('ERR_CONNECTION_REFUSED') ||
            message.includes('SecurityError') ||
            message.includes('Script origin does not match')) {
            return; // Suppress these specific errors
        }
        originalConsoleError.apply(console, args);
    };
}
</script>`

	// Insert the script right after the opening <head> tag
	re := regexp.MustCompile(`(<head[^>]*>)`)
	return re.ReplaceAllString(htmlContent, "$1\n"+suppressionScript)
}