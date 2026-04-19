package provincial

import "net/http"

func CrawlSaskatchewanMinutesLinks(archiveURL string, client *http.Client) ([]string, error) {
	return crawlSaskatchewanMinutesLinks(archiveURL, client)
}

func CrawlSaskatchewanMinutes(minutesURL string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlSaskatchewanMinutes(minutesURL, legislature, session, client)
}
