package provincial

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var nsAgendaDateRe = regexp.MustCompile(`/calendar/agenda/(\d{4}-\d{2}-\d{2})`)
var nbFullDateRe = regexp.MustCompile(`class="full-date"><span>([^<]+)</span>`)

// NovaScotiaCalendarDates extracts NS sitting dates from monthly calendar pages.
func NovaScotiaCalendarDates(client *http.Client, year int) ([]string, bool) {
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for i := -1; i <= 6; i++ {
		month := now.AddDate(0, i, 0)
		var url string
		if i == 0 {
			url = "https://nslegislature.ca/get-involved/calendar"
		} else {
			url = fmt.Sprintf("https://nslegislature.ca/get-involved/calendar/month/%d-%02d", month.Year(), month.Month())
		}
		body, err := fetchCalendarPage(client, url)
		if err != nil {
			continue
		}
		for _, m := range nsAgendaDateRe.FindAllStringSubmatch(body, -1) {
			t, err := time.Parse("2006-01-02", m[1])
			if err != nil || t.Year() != year {
				continue
			}
			seen[m[1]] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

// NewBrunswickCalendarDates extracts NB sitting dates from monthly calendar pages.
func NewBrunswickCalendarDates(client *http.Client, year int) ([]string, bool) {
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for i := -1; i <= 5; i++ {
		month := now.AddDate(0, i, 0)
		url := fmt.Sprintf("https://www.legnb.ca/en/calendar/%d-%d", month.Year(), int(month.Month()))
		body, err := fetchCalendarPage(client, url)
		if err != nil {
			continue
		}
		for _, d := range ParseNBCalendarHTML(body, year) {
			seen[d] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, true
}

// ParseNBCalendarHTML parses NB full-date calendar spans into ISO dates.
func ParseNBCalendarHTML(html string, year int) []string {
	parts := strings.Split(html, `class="calendar-cell`)
	var dates []string
	for _, part := range parts[1:] {
		quoteIdx := strings.Index(part, `"`)
		if quoteIdx < 0 {
			continue
		}
		classSuffix := part[:quoteIdx]
		if strings.Contains(classSuffix, "empty") || strings.Contains(classSuffix, "placeholder") {
			continue
		}
		m := nbFullDateRe.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		t, err := time.Parse("January 2, 2006", strings.TrimSpace(m[1]))
		if err != nil || t.Year() != year {
			continue
		}
		dates = append(dates, t.Format("2006-01-02"))
	}
	return dates
}

func fetchCalendarPage(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OpenDemocracyCrawler/1.0; +https://github.com/philspins/open-democracy)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-CA,en;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
