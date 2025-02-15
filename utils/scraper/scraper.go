package scraper

import (
	"fmt"
	"strings"

	"github.com/gocolly/colly/v2"
)

// ScrapedData represents the structured data extracted from a webpage
type ScrapedData struct {
	URL         string
	Title       string
	Text        []string
	Links       []string
	StatusCode  int
	ContentType string
}

// Scraper provides web scraping functionality
type Scraper struct {
	collector      *colly.Collector
	allowedDomains []string
}

// NewScraper creates a new scraper instance with default configuration
func NewScraper() *Scraper {
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.UserAgent("Mozilla/5.0 (compatible; Comanda/1.0; +http://github.com/kris-hansen/comanda)"),
		colly.AllowedDomains(), // Empty list means all domains are allowed
	)

	return &Scraper{
		collector: c,
	}
}

// Scrape performs web scraping on the given URL and returns structured data
func (s *Scraper) Scrape(url string) (*ScrapedData, error) {
	data := &ScrapedData{
		URL: url,
	}

	// Extract page title
	s.collector.OnHTML("title", func(e *colly.HTMLElement) {
		data.Title = e.Text
	})

	// Extract text content from paragraphs
	s.collector.OnHTML("p", func(e *colly.HTMLElement) {
		if text := e.Text; text != "" {
			data.Text = append(data.Text, text)
		}
	})

	// Extract links
	s.collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		if link != "" {
			data.Links = append(data.Links, link)
		}
	})

	// Store response info
	s.collector.OnResponse(func(r *colly.Response) {
		data.StatusCode = r.StatusCode
		data.ContentType = r.Headers.Get("Content-Type")
	})

	err := s.collector.Visit(url)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SetCustomHeaders allows setting custom headers for the scraper
func (s *Scraper) SetCustomHeaders(headers map[string]string) {
	for key, value := range headers {
		s.collector.OnRequest(func(r *colly.Request) {
			r.Headers.Set(key, value)
		})
	}
}

// AllowedDomains sets the allowed domains for scraping
func (s *Scraper) AllowedDomains(domains ...string) {
	s.allowedDomains = domains
	s.collector.OnRequest(func(r *colly.Request) {
		// If allowed domains list is not empty, check the request's host
		if len(s.allowedDomains) > 0 {
			host := r.URL.Host
			allowed := false
			for _, domain := range s.allowedDomains {
				// Check for an exact match or for the allowed domain as a suffix (to allow subdomains)
				if host == domain || strings.HasSuffix(host, "."+domain) {
					allowed = true
					break
				}
			}
			if !allowed {
				fmt.Printf("[SCRAPER] Request aborted due to disallowed domain: %s\n", host)
				r.Abort()
				return
			}
		}
	})
	fmt.Printf("[SCRAPER] Set allowed domains to: %v\n", domains)
}
