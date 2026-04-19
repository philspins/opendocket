package provincial

import "net/http"

func CrawlGenericProvincialVotes(indexURL, provinceCode, chamber string, legislature, session int, client *http.Client) ([]ProvincialDivisionResult, error) {
	return crawlGenericProvincialVotes(indexURL, provinceCode, chamber, legislature, session, client)
}
