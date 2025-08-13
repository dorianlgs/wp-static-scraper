package assets

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	
	"wp-static-scraper/utils"
)


// DownloadJob represents a single download task
type DownloadJob struct {
	URL          string
	Type         string // "css", "js", "image", "font"
	OriginalPath string // for HTML replacement
	BaseURL      *url.URL
	RetryCount   int    // Number of times this job has been retried
}

// DownloadResult contains the result of a download operation
type DownloadResult struct {
	Job       DownloadJob
	LocalPath string
	Success   bool
	Error     error
}

// ConcurrentDownloader manages parallel downloads with a worker pool
type ConcurrentDownloader struct {
	MaxWorkers    int
	jobs          chan DownloadJob
	results       chan DownloadResult
	wg            sync.WaitGroup
	totalJobs     int64
	completedJobs int64
	client        *http.Client
}

// NewConcurrentDownloader creates a new concurrent downloader
func NewConcurrentDownloader(maxWorkers int) *ConcurrentDownloader {
	// Create HTTP client with connection pooling
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: maxWorkers,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	
	return &ConcurrentDownloader{
		MaxWorkers: maxWorkers,
		jobs:       make(chan DownloadJob, maxWorkers*4), // Buffer for better performance
		results:    make(chan DownloadResult, maxWorkers*4),
		client:     client,
	}
}

// Start initializes and starts the worker pool
func (cd *ConcurrentDownloader) Start() {
	for i := 0; i < cd.MaxWorkers; i++ {
		cd.wg.Add(1)
		go cd.worker()
	}
}

// AddJob queues a download job
func (cd *ConcurrentDownloader) AddJob(job DownloadJob) {
	atomic.AddInt64(&cd.totalJobs, 1)
	cd.jobs <- job
}

// FinishJobs signals that no more jobs will be added
func (cd *ConcurrentDownloader) FinishJobs() {
	close(cd.jobs)
}

// GetResults collects all download results
func (cd *ConcurrentDownloader) GetResults() map[string]string {
	// Wait for all workers to finish
	go func() {
		cd.wg.Wait()
		close(cd.results)
	}()

	urlMap := make(map[string]string)
	
	// Collect results
	var successCount, failCount int
	for result := range cd.results {
		if result.Success {
			urlMap[result.Job.OriginalPath] = result.LocalPath
			successCount++
		} else {
			failCount++
			if result.Error != nil {
				// Only print failures for primary assets (not fonts which we expect to fail)
				if result.Job.Type != "font" {
					fmt.Printf("PRIMARY ASSET FAILED: %s (type: %s): %v\n", result.Job.URL, result.Job.Type, result.Error)
				}
			}
		}
	}
	
	return urlMap
}

// GetProgress returns current download progress
func (cd *ConcurrentDownloader) GetProgress() (completed, total int64) {
	return atomic.LoadInt64(&cd.completedJobs), atomic.LoadInt64(&cd.totalJobs)
}

// worker processes download jobs from the job queue
func (cd *ConcurrentDownloader) worker() {
	defer cd.wg.Done()
	
	for job := range cd.jobs {
		result := cd.processJob(job)
		
		// Handle retry logic without blocking
		if !result.Success && job.RetryCount < 3 {
			job.RetryCount++
			// Re-queue the job for retry
			go func(retryJob DownloadJob) {
				// Small delay before retry
				time.Sleep(time.Duration(retryJob.RetryCount) * 200 * time.Millisecond)
				cd.jobs <- retryJob
			}(job)
			continue
		}
		
		atomic.AddInt64(&cd.completedJobs, 1)
		cd.results <- result
	}
}

// processJob handles a single download job
func (cd *ConcurrentDownloader) processJob(job DownloadJob) DownloadResult {
	var localPath string
	var err error
	
	switch job.Type {
	case "css", "js", "json":
		localPath, err = cd.downloadResource(job.URL, job.Type, job.BaseURL)
	case "image":
		localPath, err = cd.downloadImage(job.URL)
	case "font":
		localPath, err = cd.downloadFont(job.URL)
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}
	
	if err != nil {
		return DownloadResult{
			Job:     job,
			Success: false,
			Error:   err,
		}
	}
	
	return DownloadResult{
		Job:       job,
		LocalPath: localPath,
		Success:   true,
	}
}

// downloadFont downloads a font file using the shared HTTP client
func (cd *ConcurrentDownloader) downloadFont(fontURL string) (string, error) {
	resp, err := cd.client.Get(fontURL)
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
	
	u, err := url.Parse(fontURL)
	if err != nil {
		return "", err
	}
	
	segments := strings.Split(u.Path, "/")
	filename := segments[len(segments)-1]
	
	// Ensure output/assets/fonts directory exists
	fontDir := "output/assets/fonts/"
	os.MkdirAll(fontDir, 0755)
	
	localPath := fontDir + filename
	
	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	
	return localPath, nil
}

// downloadImage downloads an image using the shared HTTP client
func (cd *ConcurrentDownloader) downloadImage(imageURL string) (string, error) {
	resp, err := cd.client.Get(imageURL)
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

// downloadResource downloads a resource (CSS, JS) using the shared HTTP client
func (cd *ConcurrentDownloader) downloadResource(resourceURL, ext string, base *url.URL) (string, error) {
	resp, err := cd.client.Get(resourceURL)
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
	
	// If JS, process embedded URLs and remove source map references
	if ext == "js" {
		jsContent := string(data)
		// Process JavaScript for embedded resource URLs (like template CSS files)
		jsContent, err = LocalizeJavaScriptURLs(jsContent, base)
		if err != nil {
			return "", err
		}
		// Remove source map references
		jsContent = utils.RemoveSourceMapReferences(jsContent)
		data = []byte(jsContent)
	}
	
	err = os.WriteFile(localPath, data, 0644)
	if err != nil {
		return "", err
	}
	
	return localPath, nil
}

// ProgressReporter provides real-time progress updates
type ProgressReporter struct {
	downloader *ConcurrentDownloader
	ticker     *time.Ticker
	done       chan struct{}
}

// NewProgressReporter creates a progress reporter that updates every interval
func NewProgressReporter(downloader *ConcurrentDownloader, interval time.Duration) *ProgressReporter {
	return &ProgressReporter{
		downloader: downloader,
		ticker:     time.NewTicker(interval),
		done:       make(chan struct{}),
	}
}

// Start begins progress reporting
func (pr *ProgressReporter) Start() {
	go func() {
		for {
			select {
			case <-pr.ticker.C:
				// Progress reporting disabled for better performance
			case <-pr.done:
				return
			}
		}
	}()
}

// Stop stops progress reporting
func (pr *ProgressReporter) Stop() {
	pr.ticker.Stop()
	close(pr.done)
	// Print final newline
	fmt.Println()
}