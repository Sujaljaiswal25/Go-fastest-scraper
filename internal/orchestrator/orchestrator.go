package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sujal/go-scraper/internal/crawl"
	"github.com/sujal/go-scraper/internal/fetch"
	"github.com/sujal/go-scraper/internal/output"
	"github.com/sujal/go-scraper/internal/ratelimit"
	"github.com/sujal/go-scraper/internal/types"
	"github.com/sujal/go-scraper/internal/worker"
)

// Orchestrator manages resources and dispatches scraper workflows
type Orchestrator struct {
	pool    *fetch.BrowserPool
	limiter *ratelimit.PerDomainLimiter
	cfg     types.Config
}

// New creates an Orchestrator with initialized shared resources
func New(cfg types.Config) *Orchestrator {
	var pool *fetch.BrowserPool
	// Only launch persistent Chrome pool if not in FastMode
	if cfg.BrowserPoolSize > 0 && !cfg.FastMode {
		pool = fetch.NewBrowserPool(cfg.BrowserPoolSize, !cfg.NoDaemon)
		// Connect to the browser daemon now, in parallel with the first HTTP probes
		pool.Warm()
	}

	rps := 100.0
	if cfg.DelayBetweenRequests > 0 {
		rps = 1.0 / cfg.DelayBetweenRequests.Seconds()
	}
	limiter := ratelimit.New(rps, 20)

	return &Orchestrator{
		pool:    pool,
		limiter: limiter,
		cfg:     cfg,
	}
}

// Close releases browser pool and resources
func (o *Orchestrator) Close() {
	fetch.SaveDomainCache(o.cfg.OutputDir)
	if o.pool != nil {
		o.pool.Shutdown()
	}
}

// RunScrape executes concurrent scraping across a list of URLs
func (o *Orchestrator) RunScrape(ctx context.Context, urls []string) error {
	start := time.Now()
	slog.Info("Starting scrape job", "urls_count", len(urls), "concurrency", o.cfg.Concurrency)

	wPool := worker.New(o.cfg.Concurrency, func(c context.Context, job types.Job) types.WorkerResult {
		if parsed, err := url.Parse(job.URL); err == nil {
			_ = o.limiter.Wait(c, parsed.Hostname())
		}
		res := fetch.FetchWithEscalation(c, job.URL, o.pool, &o.cfg)
		return types.WorkerResult{
			Job:    job,
			Result: *res,
			Links:  res.Links,
		}
	})

	wPool.Start(ctx)
	writer := output.NewAsyncWriter(o.cfg.OutputDir, o.cfg.OutputFormats, 4)

	// Feed jobs
	go func() {
		for _, u := range urls {
			wPool.Submit(types.Job{URL: u, Depth: 0})
		}
		wPool.Close()
	}()

	var allResults []types.ScrapeResult
	for wr := range wPool.Results() {
		allResults = append(allResults, wr.Result)
		writer.Save(wr.Result)
		status := "FAILED"
		if wr.Result.Success {
			status = fmt.Sprintf("OK [%s] in %dms", wr.Result.Method, wr.Result.Timing.TotalMs)
			if wr.Result.Cached {
				status = fmt.Sprintf("OK [cached %s] in %dms", wr.Result.Method, wr.Result.Timing.TotalMs)
			}
		}
		if !wr.Result.Success {
			status = "FAILED: " + wr.Result.Error
		}
		slog.Info(fmt.Sprintf("[%s] %s -> %s", status, wr.Result.URL, wr.Result.Title))
	}
	writer.Close()

	// Write batch reports
	o.finalizeReports(allResults, start)
	return nil
}

// RunCrawl executes BFS crawl starting from startURL
func (o *Orchestrator) RunCrawl(ctx context.Context, startURL string) error {
	start := time.Now()
	slog.Info("Starting BFS crawl", "start_url", startURL, "max_depth", o.cfg.MaxDepth, "max_pages", o.cfg.MaxPages)

	// Stream each page to disk as soon as it is crawled instead of after the whole crawl
	writer := output.NewAsyncWriter(o.cfg.OutputDir, o.cfg.OutputFormats, 4)
	crawlResults := crawl.CrawlSite(ctx, startURL, o.pool, o.limiter, o.cfg, func(cr types.CrawlPageResult) {
		writer.Save(cr.ScrapeResult)
		status := "FAILED: " + cr.Error
		if cr.Success {
			method := cr.Method
			if cr.Cached {
				method = "cached " + method
			}
			status = fmt.Sprintf("OK (depth %d) [%s] in %dms", cr.Depth, method, cr.Timing.TotalMs)
		}
		slog.Info(fmt.Sprintf("[%s] %s -> %s", status, cr.URL, cr.Title))
	})
	writer.Close()

	var scrapeResults []types.ScrapeResult
	for _, cr := range crawlResults {
		scrapeResults = append(scrapeResults, cr.ScrapeResult)
	}

	indexFile := output.SaveCrawlIndex(crawlResults, startURL, o.cfg.OutputDir)
	slog.Info("Crawl index generated", "file", indexFile)

	o.finalizeReports(scrapeResults, start)
	return nil
}

// RunSitemap discovers URLs from sitemap and scrapes them concurrently
func (o *Orchestrator) RunSitemap(ctx context.Context, baseURL string) error {
	slog.Info("Discovering sitemap URLs", "base_url", baseURL)
	entries, err := crawl.DiscoverSitemapURLs(ctx, baseURL)
	if err != nil {
		return fmt.Errorf("sitemap discovery error: %w", err)
	}

	if len(entries) == 0 {
		slog.Warn("No sitemap URLs discovered. Falling back to single-URL scrape", "url", baseURL)
		return o.RunScrape(ctx, []string{baseURL})
	}

	var urls []string
	limit := o.cfg.MaxPages
	if limit <= 0 {
		limit = len(entries)
	}
	for i, e := range entries {
		if i >= limit {
			break
		}
		urls = append(urls, e.URL)
	}

	slog.Info(fmt.Sprintf("Discovered %d sitemap URLs, scraping %d", len(entries), len(urls)))
	return o.RunScrape(ctx, urls)
}

// RunDiscover discovers URLs from sitemap, prints them, and saves to sitemap.json without scraping
func (o *Orchestrator) RunDiscover(ctx context.Context, baseURL string) error {
	slog.Info("Discovering sitemap URLs (dry-run)", "base_url", baseURL)
	entries, err := crawl.DiscoverSitemapURLs(ctx, baseURL)
	if err != nil {
		return fmt.Errorf("sitemap discovery error: %w", err)
	}

	_ = output.EnsureDir(o.cfg.OutputDir)
	jsonPath := filepath.Join(o.cfg.OutputDir, "discovered_urls.json")
	data, _ := json.MarshalIndent(entries, "", "  ")
	_ = os.WriteFile(jsonPath, data, 0644)

	fmt.Printf("\n✨ Discovered %d URLs from %s:\n", len(entries), baseURL)
	for i, e := range entries {
		if i < 25 {
			fmt.Printf("  [%d] %s\n", i+1, e.URL)
		}
	}
	if len(entries) > 25 {
		fmt.Printf("  ... and %d more (see %s)\n", len(entries)-25, jsonPath)
	}

	return nil
}

func (o *Orchestrator) finalizeReports(results []types.ScrapeResult, start time.Time) {
	elapsed := time.Since(start)

	// Check if CSV was requested
	hasCSV := false
	for _, f := range o.cfg.OutputFormats {
		if strings.ToLower(strings.TrimSpace(f)) == "csv" {
			hasCSV = true
			break
		}
	}
	if hasCSV {
		csvFile, err := output.SaveCSV(results, o.cfg.OutputDir)
		if err == nil {
			slog.Info("CSV report saved", "file", csvFile)
		}
	}

	summaryFile := output.SaveSummary(results, elapsed, o.cfg.OutputDir)
	slog.Info("Summary report saved", "file", summaryFile, "elapsed", elapsed.Round(time.Millisecond))
}
