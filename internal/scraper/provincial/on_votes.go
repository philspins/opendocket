package provincial

import "net/http"

func CrawlOntarioVPSittingDates(indexURL string, parliament, session int, client *http.Client) ([]string, error) {
	return crawlOntarioVPSittingDates(indexURL, parliament, session, client)
}

func OntarioVPDayURL(parliament, session int, date string) string {
	return ontarioVPDayURL(parliament, session, date)
}

func CrawlOntarioVPDay(vpURL string, parliament, session int, date string, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlOntarioVPDay(vpURL, parliament, session, date, client)
}
