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

// Priority levels for download jobs
type Priority int

const (
	HighPriority Priority = iota // CSS, JS, JSON - critical for page rendering
	LowPriority                  // Images, fonts - can be loaded after critical assets
)

// DownloadJob represents a single download task
type DownloadJob struct {
	URL          string
	Type         string // "css", "js", "image", "font"
	OriginalPath string // for HTML replacement
	BaseURL      *url.URL
	Priority     Priority // Priority level for this job
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
	MaxWorkers          int
	highPriorityJobs    chan DownloadJob
	lowPriorityJobs     chan DownloadJob
	results             chan DownloadResult
	wg                  sync.WaitGroup
	done                chan struct{}
	totalJobs           int
	highPriorityJobsCount int
	lowPriorityJobsCount  int
	completedJobs       int
	mu                  sync.Mutex
}

// NewConcurrentDownloader creates a new concurrent downloader
func NewConcurrentDownloader(maxWorkers int) *ConcurrentDownloader {
	return &ConcurrentDownloader{
		MaxWorkers:       maxWorkers,
		highPriorityJobs: make(chan DownloadJob, maxWorkers*2), // Buffer for better performance
		lowPriorityJobs:  make(chan DownloadJob, maxWorkers*2),
		results:          make(chan DownloadResult, maxWorkers*2),
		done:             make(chan struct{}),
	}
}

// Start initializes and starts the worker pool
func (cd *ConcurrentDownloader) Start() {
	for i := 0; i < cd.MaxWorkers; i++ {
		cd.wg.Add(1)
		go cd.worker()
	}
}

// AddJob queues a download job, routing to appropriate priority queue
func (cd *ConcurrentDownloader) AddJob(job DownloadJob) {
	// Auto-assign priority based on asset type if not explicitly set
	if job.Priority == 0 { // Priority 0 means not set, assign based on type
		switch job.Type {
		case "css", "js", "json":
			job.Priority = HighPriority
		case "image", "font":
			job.Priority = LowPriority
		default:
			job.Priority = LowPriority // Default to low priority for unknown types
		}
	}
	
	cd.mu.Lock()
	cd.totalJobs++
	if job.Priority == HighPriority {
		cd.highPriorityJobsCount++
	} else {
		cd.lowPriorityJobsCount++
	}
	cd.mu.Unlock()
	
	// Route to appropriate priority queue
	if job.Priority == HighPriority {
		cd.highPriorityJobs <- job
	} else {
		cd.lowPriorityJobs <- job
	}
}

// FinishJobs signals that no more jobs will be added
func (cd *ConcurrentDownloader) FinishJobs() {
	close(cd.highPriorityJobs)
	close(cd.lowPriorityJobs)
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

// GetDetailedProgress returns priority-specific download progress
func (cd *ConcurrentDownloader) GetDetailedProgress() (completed, total, highPriority, lowPriority int) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.completedJobs, cd.totalJobs, cd.highPriorityJobsCount, cd.lowPriorityJobsCount
}

// worker processes download jobs from priority queues
func (cd *ConcurrentDownloader) worker() {
	defer cd.wg.Done()
	
	for {
		var job DownloadJob
		var ok bool
		
		// Priority-aware job selection: check high priority first, then low priority
		select {
		case job, ok = <-cd.highPriorityJobs:
			if !ok {
				// High priority queue closed, check if low priority queue is also closed
				select {
				case job, ok = <-cd.lowPriorityJobs:
					if !ok {
						// Both queues closed, worker exits
						return
					}
				default:
					// Low priority queue empty and high priority closed, worker exits
					return
				}
			}
		default:
			// No high priority jobs available, check low priority
			select {
			case job, ok = <-cd.lowPriorityJobs:
				if !ok {
					// Low priority queue closed, worker exits
					return
				}
			case job, ok = <-cd.highPriorityJobs:
				// High priority job arrived while waiting, prioritize it
				if !ok {
					// High priority queue closed, check low priority one more time
					select {
					case job, ok = <-cd.lowPriorityJobs:
						if !ok {
							return
						}
					default:
						return
					}
				}
			}
		}
		
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
				completed, total, highPriority, lowPriority := pr.downloader.GetDetailedProgress()
				if total > 0 {
					fmt.Printf("\rDownloading assets: %d/%d (%.1f%%) [High: %d, Low: %d]", 
						completed, total, float64(completed)/float64(total)*100, highPriority, lowPriority)
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