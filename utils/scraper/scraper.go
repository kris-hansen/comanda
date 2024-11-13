package scraper

import (
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
	collector *colly.Collector
}

// NewScraper creates a new scraper instance with default configuration
func NewScraper() *Scraper {
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		colly.UserAgent("Mozilla/5.0 (compatible; Comanda/1.0; +http://github.com/kris-hansen/comanda)"),
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
	s.collector.AllowedDomains = domains
}
