package assets

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// DownloadJob represents a single download task
type DownloadJob struct {
	URL          string
	Type         string // "css", "js", "image", "font"
	OriginalPath string // for HTML replacement
	BaseURL      *url.URL
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
	done          chan struct{}
	totalJobs     int
	completedJobs int
	mu            sync.Mutex
}

// NewConcurrentDownloader creates a new concurrent downloader
func NewConcurrentDownloader(maxWorkers int) *ConcurrentDownloader {
	return &ConcurrentDownloader{
		MaxWorkers: maxWorkers,
		jobs:       make(chan DownloadJob, maxWorkers*2), // Buffer for better performance
		results:    make(chan DownloadResult, maxWorkers*2),
		done:       make(chan struct{}),
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
	cd.mu.Lock()
	cd.totalJobs++
	cd.mu.Unlock()
	
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
		close(cd.done)
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
func (cd *ConcurrentDownloader) GetProgress() (completed, total int) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.completedJobs, cd.totalJobs
}

// worker processes download jobs from the job queue
func (cd *ConcurrentDownloader) worker() {
	defer cd.wg.Done()
	
	for job := range cd.jobs {
		result := cd.processJob(job)
		
		cd.mu.Lock()
		cd.completedJobs++
		cd.mu.Unlock()
		
		cd.results <- result
	}
}

// processJob handles a single download job with retry logic
func (cd *ConcurrentDownloader) processJob(job DownloadJob) DownloadResult {
	var lastErr error
	maxRetries := 3
	
	for attempt := 0; attempt < maxRetries; attempt++ {
		var localPath string
		var err error
		
		switch job.Type {
		case "css", "js", "json":
			localPath, err = DownloadResource(job.URL, job.Type, job.BaseURL)
		case "image":
			localPath, err = DownloadImage(job.URL)
		case "font":
			localPath, err = downloadFont(job.URL)
		default:
			err = fmt.Errorf("unknown job type: %s", job.Type)
		}
		
		if err == nil {
			return DownloadResult{
				Job:       job,
				LocalPath: localPath,
				Success:   true,
			}
		}
		
		lastErr = err
		
		// Exponential backoff for retries
		if attempt < maxRetries-1 {
			backoff := time.Duration(attempt+1) * 500 * time.Millisecond
			time.Sleep(backoff)
		}
	}
	
	return DownloadResult{
		Job:     job,
		Success: false,
		Error:   lastErr,
	}
}

// downloadFont downloads a font file and saves it to the fonts directory
func downloadFont(fontURL string) (string, error) {
	resp, err := http.Get(fontURL)
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
				completed, total := pr.downloader.GetProgress()
				if total > 0 {
					fmt.Printf("\rDownloading assets: %d/%d (%.1f%%)", 
						completed, total, float64(completed)/float64(total)*100)
				}
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