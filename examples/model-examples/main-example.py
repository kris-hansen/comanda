package main

import (
	"fmt"
	"log"
	"your_project_path/input"
)

func main() {
	url := "http://example.com"
	config := map[string]interface{}{
		"allowed_domains": []string{"example.com"},
		"headers": map[string]string{
			"User-Agent": "Mozilla/5.0",
		},
		"extract": []string{"title", "meta"},
	}

	scrapeInput, err := input.ProcessScrape(url, config)
	if err != nil {
		log.Fatalf("Error processing scrape input: %v", err)
	}import requests

	def fetch_data(url, headers):
		response = requests.get(url, headers=headers)
		if response.status_code == 200:
			return response.text
		else:
			response.raise_for_status()
	
	def main():
		url = "http://example.com"
		headers = {
			"User-Agent": "Mozilla/5.0"
		}
	
		try:
			data = fetch_data(url, headers)
			print("Fetched data:", data[:100])  # Print first 100 characters of the data
		except requests.RequestException as e:
			print(f"Error fetching data: {e}")
	
	if __name__ == "__main__":
		main()

	log.Printf("Scrape Input: %+v\n", scrapeInput)
}
