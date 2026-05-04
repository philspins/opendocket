package provincial

import (
	"net/http"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/philspins/opendocket/internal/clog"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const wikiBase = "https://en.wikipedia.org"

// provincialWikiCategoryPaths maps province code to one or more Wikipedia
// category paths. Both century variants are listed to cover former members
// elected before 2000.
var provincialWikiCategoryPaths = map[string][]string{
	"ab": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_Alberta",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_Alberta",
	},
	"bc": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_British_Columbia",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_British_Columbia",
	},
	"mb": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_Manitoba",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_Manitoba",
	},
	"nb": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_New_Brunswick",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_New_Brunswick",
	},
	"nl": {
		"/wiki/Category:21st-century_members_of_the_House_of_Assembly_of_Newfoundland_and_Labrador",
		"/wiki/Category:20th-century_members_of_the_House_of_Assembly_of_Newfoundland_and_Labrador",
	},
	"ns": {
		"/wiki/Category:21st-century_members_of_the_Nova_Scotia_House_of_Assembly",
		"/wiki/Category:20th-century_members_of_the_Nova_Scotia_House_of_Assembly",
	},
	"on": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_Ontario",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_Ontario",
	},
	"pe": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_Prince_Edward_Island",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_Prince_Edward_Island",
	},
	"qc": {
		"/wiki/Category:21st-century_members_of_the_National_Assembly_of_Quebec",
		"/wiki/Category:20th-century_members_of_the_National_Assembly_of_Quebec",
	},
	"sk": {
		"/wiki/Category:21st-century_members_of_the_Legislative_Assembly_of_Saskatchewan",
		"/wiki/Category:20th-century_members_of_the_Legislative_Assembly_of_Saskatchewan",
	},
}

type wikiEntry struct {
	name string
	url  string
}

type wikiMemberInfo struct {
	party  string
	riding string
}

// provincialWikiLookup caches Wikipedia category pages and individual article
// scrapes for a single province. Created once per crawl run; loads lazily.
// baseURL defaults to https://en.wikipedia.org; override in tests.
type provincialWikiLookup struct {
	baseURL       string
	categoryPaths []string
	client        *http.Client
	byNormSurname map[string][]wikiEntry
	articles      map[string]wikiMemberInfo
	loaded        bool
}

// newProvincialWikiLookup returns a lookup for the given province code, or nil
// if that province has no Wikipedia category configured.
func newProvincialWikiLookup(provinceCode string, client *http.Client) *provincialWikiLookup {
	paths := provincialWikiCategoryPaths[provinceCode]
	if len(paths) == 0 {
		return nil
	}
	return &provincialWikiLookup{
		baseURL:       wikiBase,
		categoryPaths: paths,
		client:        client,
		byNormSurname: make(map[string][]wikiEntry),
		articles:      make(map[string]wikiMemberInfo),
	}
}

func foldAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return result
}

func (w *provincialWikiLookup) ensureLoaded() {
	if w.loaded || w.client == nil {
		return
	}
	w.loaded = true
	total := 0
	for _, path := range w.categoryPaths {
		categoryURL := w.baseURL + path
		doc, err := fetchDoc(categoryURL, w.client)
		if err != nil {
			clog.Debugf("[wiki] category fetch %s: %v", categoryURL, err)
			continue
		}
		doc.Find("#mw-pages a[href]").Each(func(_ int, a *goquery.Selection) {
			href := a.AttrOr("href", "")
			if !strings.HasPrefix(href, "/wiki/") || strings.Contains(href[6:], ":") {
				return
			}
			text := strings.TrimSpace(a.Text())
			if text == "" {
				return
			}
			articleURL := w.baseURL + href
			parts := strings.Fields(normalisePersonName(text))
			if len(parts) == 0 {
				return
			}
			key := strings.ToLower(foldAccents(parts[len(parts)-1]))
			// Skip duplicates that appear in both century categories.
			for _, existing := range w.byNormSurname[key] {
				if existing.url == articleURL {
					return
				}
			}
			w.byNormSurname[key] = append(w.byNormSurname[key], wikiEntry{name: text, url: articleURL})
			total++
		})
	}
	clog.Debugf("[wiki] loaded %d entries across %d category pages", total, len(w.categoryPaths))
}

// lookup returns party and riding for a member name from a provincial vote record.
// The name is typically a bare surname or "First Last" after title stripping.
func (w *provincialWikiLookup) lookup(memberName string) (party, riding string, ok bool) {
	w.ensureLoaded()
	if len(w.byNormSurname) == 0 {
		return "", "", false
	}

	normName := normalisePersonName(memberName)
	parts := strings.Fields(normName)
	if len(parts) == 0 {
		return "", "", false
	}
	surname := strings.ToLower(foldAccents(parts[len(parts)-1]))
	entries := w.byNormSurname[surname]
	if len(entries) == 0 {
		return "", "", false
	}

	articleURL := entries[0].url
	if len(entries) > 1 && len(parts) >= 2 {
		// Disambiguate by first name when multiple people share a surname.
		first := strings.ToLower(foldAccents(parts[0]))
		for _, e := range entries {
			eParts := strings.Fields(strings.ToLower(foldAccents(normalisePersonName(e.name))))
			if len(eParts) > 0 && strings.HasPrefix(eParts[0], first) {
				articleURL = e.url
				break
			}
		}
	}

	info := w.fetchArticle(articleURL)
	if info.party == "" && info.riding == "" {
		return "", "", false
	}
	return info.party, info.riding, true
}

// wikiFirstLinkOrText returns the text of the first hyperlink in sel, or the
// full trimmed text if no link exists. Using the link text avoids auto-generated
// qualifiers like "(since 2026)" that appear as trailing plain text outside links.
func wikiFirstLinkOrText(sel *goquery.Selection) string {
	if a := sel.Find("a").First(); a.Length() > 0 {
		return strings.TrimSpace(a.Text())
	}
	value := strings.TrimSpace(sel.Text())
	return strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
}

func (w *provincialWikiLookup) fetchArticle(url string) wikiMemberInfo {
	if cached, ok := w.articles[url]; ok {
		return cached
	}
	info := wikiMemberInfo{}
	doc, err := fetchDoc(url, w.client)
	if err != nil {
		clog.Debugf("[wiki] article fetch %s: %v", url, err)
		w.articles[url] = info
		return info
	}

	// fallbackRiding holds a riding extracted from a non-provincial row (e.g. a
	// federal "Member of Parliament" row). It is used only when no provincial
	// legislature row is found, since federal and provincial ridings often share
	// a name but are not always identical.
	var fallbackRiding string

	doc.Find("table.infobox tr").Each(func(_ int, row *goquery.Selection) {
		th := row.Find("th")
		td := row.Find("td").First()

		if td.Length() > 0 {
			// Standard th/td pair — use the first hyperlink text to avoid
			// qualifiers like "(since 2026)" that appear as trailing plain text.
			label := strings.ToLower(strings.TrimSpace(th.Text()))
			value := wikiFirstLinkOrText(td)
			if value == "" {
				return
			}
			switch {
			case strings.Contains(label, "political party") || label == "party":
				if info.party == "" {
					info.party = value
				}
			case strings.Contains(label, "constituency") || strings.Contains(label, "electoral district") || strings.Contains(label, "riding"):
				if info.riding == "" {
					info.riding = value
				}
			}
			return
		}

		// Full-width office header: <th colspan="2">Member of [legislature]<br/>for [Riding]</th>
		// These rows have no <td>; the riding is the last hyperlink inside the <th>.
		// Match on "member of" rather than trying to split on "for", since <br/> tags
		// between the office title and riding name may not produce a space in .Text().
		label := strings.ToLower(strings.Join(strings.Fields(th.Text()), " "))
		if !strings.Contains(label, "member of") && !strings.Contains(label, "member for") {
			return
		}
		a := th.Find("a").Last()
		if a.Length() == 0 {
			return
		}
		riding := strings.TrimSpace(a.Text())
		if riding == "" {
			return
		}
		isProvincial := strings.Contains(label, "legislative assembly") ||
			strings.Contains(label, "provincial parliament") ||
			strings.Contains(label, "house of assembly") ||
			strings.Contains(label, "national assembly")
		if isProvincial {
			if info.riding == "" {
				info.riding = riding
			}
		} else if fallbackRiding == "" {
			// e.g. federal "Member of Parliament for [Riding]" — keep as fallback
			// in case no provincial legislature row appears in this article.
			fallbackRiding = riding
		}
	})

	if info.riding == "" {
		info.riding = fallbackRiding
	}

	clog.Debugf("[wiki] %s → party=%q riding=%q", url, info.party, info.riding)
	w.articles[url] = info
	return info
}
