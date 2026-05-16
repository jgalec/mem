package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultConcurrency = 8
const defaultTimeout = 10 * time.Second

type LinkStatus string

const (
	StatusOK       LinkStatus = "ok"
	StatusBroken   LinkStatus = "broken"
	StatusRedirect LinkStatus = "redirect"
	StatusSkipped  LinkStatus = "skipped"
	StatusError    LinkStatus = "error"
)

type LinkResult struct {
	URL        string     `json:"url"`
	Status     LinkStatus `json:"status"`
	StatusCode int        `json:"status_code,omitempty"`
	Message    string     `json:"message,omitempty"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
}

type Checker struct {
	Concurrency int
	Timeout     time.Duration
}

func NewChecker() *Checker {
	return &Checker{
		Concurrency: defaultConcurrency,
		Timeout:     defaultTimeout,
	}
}

var (
	inlineLinkRe   = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	refLinkRe      = regexp.MustCompile(`^\s*\[([^\]]+)\]:\s+<([^>\s]+)>`)
	angleLinkRe    = regexp.MustCompile(`<(https?://[^>]+)>`)
	bareURLRe      = regexp.MustCompile(`(?m)(?:(?:^|\s)(https?://[^\s<>"]+))`)
	imageRe        = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
)

func ExtractLinks(content string) []struct{ URL, Text string } {
	var links []struct{ URL, Text string }
	seen := make(map[string]bool)

	for _, re := range []*regexp.Regexp{inlineLinkRe, imageRe} {
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			u := strings.TrimSpace(m[2])
			if u != "" && !seen[u] && !strings.HasPrefix(u, "#") {
				seen[u] = true
				links = append(links, struct{ URL, Text string }{URL: u, Text: m[1]})
			}
		}
	}

	for _, m := range refLinkRe.FindAllStringSubmatch(content, -1) {
		u := strings.TrimSpace(m[2])
		if u != "" && !seen[u] {
			seen[u] = true
			links = append(links, struct{ URL, Text string }{URL: u, Text: m[1]})
		}
	}

	for _, m := range bareURLRe.FindAllStringSubmatch(content, -1) {
		u := strings.TrimSpace(m[1])
		if u != "" && !seen[u] {
			seen[u] = true
			links = append(links, struct{ URL, Text string }{URL: u, Text: ""})
		}
	}

	return links
}

func (c *Checker) CheckFile(filePath string) ([]LinkResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	extracted := ExtractLinks(string(content))

	if len(extracted) == 0 {
		return nil, nil
	}

	var results []LinkResult
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.Concurrency)
	resultCh := make(chan LinkResult, len(extracted))

	for _, link := range extracted {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			lineNum := findLine(lines, u)
			result := c.CheckLink(u, filePath, lineNum)
			resultCh <- result
		}(link.URL)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for r := range resultCh {
		results = append(results, r)
	}

	return results, nil
}

func (c *Checker) CheckLink(rawURL, filePath string, lineNum int) LinkResult {
	result := LinkResult{
		URL:  rawURL,
		File: filePath,
		Line: lineNum,
	}

	if strings.HasPrefix(rawURL, "mailto:") {
		result.Status = StatusSkipped
		result.Message = "mailto link skipped"
		return result
	}

	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return c.checkHTTP(rawURL, filePath, lineNum)
	}

	if strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, ".") || (!strings.Contains(rawURL, ":") && !strings.Contains(rawURL, "//")) {
		return c.checkLocal(rawURL, filePath, lineNum)
	}

	result.Status = StatusSkipped
	result.Message = "unsupported scheme"
	return result
}

func (c *Checker) checkHTTP(rawURL, filePath string, lineNum int) LinkResult {
	result := LinkResult{URL: rawURL, File: filePath, Line: lineNum}

	client := &http.Client{
		Timeout: c.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Head(rawURL)
	if err != nil {
		parsed, parseErr := url.Parse(rawURL)
		if parseErr == nil && parsed.Scheme == "https" {
			altURL := "http://" + parsed.Host + parsed.Path
			if parsed.RawQuery != "" {
				altURL += "?" + parsed.RawQuery
			}
			if altResp, altErr := client.Head(altURL); altErr == nil {
				altResp.Body.Close()
				result.Status = StatusOK
				result.StatusCode = altResp.StatusCode
				result.Message = "ok (via http fallback)"
				return result
			}
		}
		result.Status = StatusError
		result.Message = err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = StatusOK
	} else if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		result.Status = StatusRedirect
		loc := resp.Header.Get("Location")
		if loc != "" {
			result.Message = "redirects to " + loc
		} else {
			result.Message = "redirect (no Location header)"
		}
	} else {
		result.Status = StatusBroken
		result.Message = http.StatusText(resp.StatusCode)
	}
	result.StatusCode = resp.StatusCode

	return result
}

func (c *Checker) checkLocal(rawURL, filePath string, lineNum int) LinkResult {
	result := LinkResult{URL: rawURL, File: filePath, Line: lineNum}

	base := filepath.Dir(filePath)
	target := rawURL

	if idx := strings.Index(target, "#"); idx != -1 {
		target = target[:idx]
	}

	if target == "" {
		result.Status = StatusOK
		result.Message = "same-file anchor"
		return result
	}

	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(base, target)
	}
	resolved = filepath.Clean(resolved)

	if _, err := os.Stat(resolved); os.IsNotExist(err) {
		result.Status = StatusBroken
		result.Message = "file not found: " + resolved
	} else if err != nil {
		result.Status = StatusError
		result.Message = err.Error()
	} else {
		result.Status = StatusOK
		result.Message = resolved
	}

	return result
}

func findLine(lines []string, url string) int {
	for i, line := range lines {
		if strings.Contains(line, url) {
			return i + 1
		}
	}
	return 0
}

func FindMarkdownFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".md") || strings.HasSuffix(info.Name(), ".markdown")) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func Summary(results []LinkResult) (ok, broken, redirect, error_, skipped int) {
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusBroken:
			broken++
		case StatusRedirect:
			redirect++
		case StatusError:
			error_++
		case StatusSkipped:
			skipped++
		}
	}
	return
}
