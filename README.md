# 🚀 Go Scraper — Firecrawl-Speed Web Scraper & Crawler

A high-performance, production-ready web scraper and crawler built in Go, designed for extreme throughput and Firecrawl-level speeds.

---

## ⚡ Highlights

- **Massive Goroutine Concurrency**: Schedules hundreds to thousands of concurrent lightweight workers with bounded backpressure.
- **Connection Pooling & HTTP/2**: Persistent TCP keep-alive sockets reuse handshakes across requests to the same origin.
- **Progressive 4-Tier Escalation**:
  - **Tier 1**: Sub-millisecond raw `net/http` fetch with realistic browser header rotation.
  - **Tier 2**: Headless Chrome rendering via `chromedp` tab reuse (persistent browser instance pool).
  - **Tier 3**: Stealth Chrome with runtime JS patches (navigator webdriver masking, WebGL renderer spoofing, plugins faking).
  - **Tier 4**: Stealth + auto-scroll for dynamic SPAs and infinite-scroll pages.
- **Domain Learning Cache**: Remembers winning fetch tier per domain to skip sequential fallback penalty on future URLs.
- **Dual-Engine Content Extraction**: Combines Mozilla Readability (`go-readability`) and structural DOM filtering (`goquery`).
- **Markdown & Structured Data Extraction**: Produces GitHub-flavored Markdown, JSON-LD structured blocks, metadata, and outbound link graphs.
- **BFS Website Crawler**: Concurrent breadth-first crawler with robots.txt compliance, sitemap XML discovery, and regex URL filtering.
- **Token Bucket Rate Limiting**: Per-domain rate limits powered by `golang.org/x/time/rate`.

---

## 📦 Installation & Build

```bash
# Clone the repository
git clone https://github.com/sujal/go-scraper.git
cd go-scraper

# Build the binary
go build -o bin/go-scraper.exe ./cmd/scraper
```

---

## 🛠️ Usage

### Quick Commands (Windows Batch)

```bash
# 1. Scrape any page to Markdown & JSON
.\scrape.bat https://example.com

# Scrape multiple URLs
.\scrape.bat https://news.ycombinator.com https://golang.org

# 2. BFS Crawl a website
.\crawl.bat https://docs.python.org --pages=30 --depth=2

# 3. Sitemap scrape
.\run.bat sitemap https://example.com --pages=50

# 4. Discover sitemap URLs (dry-run)
.\run.bat discover https://example.com
```

### Direct Binary Usage

```bash
# Scrape
.\go-scraper.exe scrape https://example.com

# Crawl
.\go-scraper.exe crawl https://docs.python.org --pages=50 --depth=2
```

---

## ⚙️ CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-c, --concurrency` | Number of concurrent workers | `30` |
| `-o, --output` | Destination output directory | `output` |
| `-f, --formats` | Formats: `markdown`, `json`, `text`, `html`, `csv` | `markdown,json` |
| `--fast` | Pure HTTP, never launch a browser | `false` |
| `--max-age` | Reuse a cached result if younger than this (`0` = always fetch fresh) | `48h` |
| `--browsers` | Chrome instances: 1 = the shared background daemon; more adds local instances | `1` |
| `--no-daemon` | Launch a private Chrome that exits with the run instead of using the daemon | `false` |
| `-d, --delay` | Delay between requests per domain | `0` |
| `-t, --timeout` | HTTP request timeout | `10s` |
| `--ua-rotate` | Rotate realistic browser User-Agents | `true` |
| `--depth` | Max BFS crawl depth | `3` |
| `-p, --pages` | Max pages to crawl | `50` |
| `--respect-robots` | Honor robots.txt crawl rules | `true` |
| `--include` | Regex to only scrape matching URLs | `""` |
| `--exclude` | Regex to skip matching URLs | `""` |
| `--follow-external`| Allow crawling external domains | `false` |
| `-v, --verbose` | Debug logging with per-phase browser timings | `false` |

### Background browser & cache

The first run that needs a browser starts a headless Chrome in a detached background process and later runs reuse it, so a fresh scrape connects in ~50ms instead of launching Chrome. It exits by itself after 10 minutes without work.

```bash
go-scraper browser status   # is the background Chrome running?
go-scraper browser stop     # stop it now
go-scraper cache clear      # forget all cached results (see --max-age)
```

Results are cached for 48 hours by default (same as Firecrawl's `maxAge`); a repeat scrape of the same URL returns in a few milliseconds and is logged as `[cached ...]`. Pass `--max-age 0` to always fetch fresh.

---

## 📁 Output Artifacts

- **Markdown (`.md`)**: Full formatted document with source attribution and headings.
- **JSON (`.json`)**: Rich JSON containing text, markdown, metadata, OpenGraph tags, JSON-LD, timing stats, and links.
- **Batch CSV (`results.csv`)**: Tabular export of status, timings, content length, and errors.
- **Summary Report (`SUMMARY.md`)**: Human-readable benchmark report with success rates and timing distribution.
- **Crawl Index (`CRAWL_INDEX.md`)**: Sitemap and depth graph of crawled URLs.
