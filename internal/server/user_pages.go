package server

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/opennorth"
	"github.com/philspins/opendocket/internal/riding"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/templates"
	"github.com/philspins/opendocket/internal/visitor"
)

func (s *Server) setLocalRidingCookies(w http.ResponseWriter, federalRidingID, provincialRidingID string) {
	cookies := []struct {
		name  string
		value string
	}{
		{name: visitor.FederalRidingCookie, value: strings.TrimSpace(federalRidingID)},
		{name: visitor.ProvincialRidingCookie, value: strings.TrimSpace(provincialRidingID)},
	}
	for _, c := range cookies {
		cookieValue := url.QueryEscape(c.value)
		cookie := &http.Cookie{
			Name:     c.name,
			Value:    cookieValue,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   strings.HasPrefix(strings.ToLower(s.baseURL), "https://"),
		}
		if c.value == "" {
			cookie.MaxAge = -1
		} else {
			cookie.Expires = time.Now().Add(visitor.RidingCookieTTL)
		}
		http.SetCookie(w, cookie)
	}
}

func hasLocalRidingContext(result riding.LookupResult) bool {
	return strings.TrimSpace(result.FederalRidingID) != "" || strings.TrimSpace(result.ProvincialRidingID) != ""
}

func fallbackLookupResult(federalRidingID, provincialRidingID string, st *store.Store) riding.LookupResult {
	result := riding.LookupResult{
		FederalRidingID:    strings.TrimSpace(federalRidingID),
		ProvincialRidingID: strings.TrimSpace(provincialRidingID),
	}
	if result.FederalRidingID != "" {
		result.FederalRepresentative = opennorth.Representative{
			Name:          "Current federal representative",
			ElectedOffice: "MP",
			DistrictName:  result.FederalRidingID,
		}
		members, _ := st.GetMembersByRiding(result.FederalRidingID)
		if member, ok := selectLocalMemberByLevel(members, result.FederalRidingID, "federal"); ok {
			result.FederalRepresentative = opennorth.Representative{
				Name:          member.Name,
				ElectedOffice: "MP",
				PartyName:     member.Party,
				DistrictName:  member.Riding,
				Email:         member.Email,
				URL:           member.Website,
				PhotoURL:      member.PhotoURL,
				LocalMemberID: member.ID,
			}
		}
	}
	if result.ProvincialRidingID != "" {
		result.ProvincialRepresentative = opennorth.Representative{
			Name:          "Current provincial representative",
			ElectedOffice: "Provincial representative",
			DistrictName:  result.ProvincialRidingID,
		}
		members, _ := st.GetMembersByRiding(result.ProvincialRidingID)
		if member, ok := selectLocalMemberByLevel(members, result.ProvincialRidingID, "provincial"); ok {
			result.ProvincialRepresentative = opennorth.Representative{
				Name:          member.Name,
				ElectedOffice: member.Role,
				PartyName:     member.Party,
				DistrictName:  member.Riding,
				Email:         member.Email,
				URL:           member.Website,
				PhotoURL:      member.PhotoURL,
				LocalMemberID: member.ID,
			}
		}
	}
	return result
}

func selectLocalMemberByLevel(members []store.MemberRow, ridingID, level string) (store.MemberRow, bool) {
	ridingID = strings.TrimSpace(ridingID)
	for _, member := range members {
		if !memberMatchesLevel(member, level) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(member.Riding), ridingID) {
			return member, true
		}
	}
	for _, member := range members {
		if memberMatchesLevel(member, level) {
			return member, true
		}
	}
	return store.MemberRow{}, false
}

func memberMatchesLevel(member store.MemberRow, level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "provincial":
		if strings.EqualFold(member.GovernmentLevel, "provincial") {
			return true
		}
		chamber := strings.ToLower(strings.TrimSpace(member.Chamber))
		return chamber != "" && chamber != "commons" && chamber != "senate"
	default:
		if strings.EqualFold(member.GovernmentLevel, "federal") {
			return true
		}
		chamber := strings.ToLower(strings.TrimSpace(member.Chamber))
		return chamber == "" || chamber == "commons" || chamber == "senate"
	}
}

func (s *Server) handleRiding(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	ps := s.parliamentStatus()
	var (
		reps      []opennorth.Representative
		lookupErr string
	)

	if address != "" {
		result, err := s.riding.Lookup(r.Context(), address)
		if err != nil {
			log.Printf("handleRiding lookup failed for %q: %v", address, err)
			switch {
			case strings.Contains(err.Error(), "missing GOOGLE_MAPS_API_KEY"):
				lookupErr = "Address lookup is not configured (missing GOOGLE_MAPS_API_KEY)."
			case strings.HasPrefix(err.Error(), "representatives:"):
				lookupErr = "Could not look up representatives. Please try again."
			default:
				lookupErr = "Could not locate that address. Please try a more specific Canadian address."
			}
		} else {
			reps = result.Representatives
			s.setLocalRidingCookies(w, result.FederalRidingID, result.ProvincialRidingID)
			if user, ok := s.auth.SessionUser(r); ok {
				if _, saveErr := s.store.UpdateUserLocation(user.ID, result.FederalRidingID, result.ProvincialRidingID); saveErr != nil {
					log.Printf("handleRiding save failed for user=%q: %v", user.ID, saveErr)
				}
			}
		}
	}

	_ = templates.RidingLookup(ps, address, reps, lookupErr, s.riding.PlacesAPIKey()).Render(r.Context(), w)
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.auth.SessionUser(r)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderProfile(w, r, user, "", "", r.URL.Query().Get("updated") == "1")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		address := strings.TrimSpace(r.FormValue("address"))
		federalRiding := strings.TrimSpace(r.FormValue("federal_riding"))
		provincialRiding := strings.TrimSpace(r.FormValue("provincial_riding"))

		if address != "" {
			// Address-based lookup path (existing behaviour).
			result, err := s.riding.Lookup(r.Context(), address)
			if err != nil {
				lookupErr := "Could not locate that address. Please try a more specific Canadian address."
				if strings.Contains(err.Error(), "missing GOOGLE_MAPS_API_KEY") {
					lookupErr = "Address lookup is not configured (missing GOOGLE_MAPS_API_KEY)."
				} else if strings.HasPrefix(err.Error(), "representatives:") {
					lookupErr = "Could not look up representatives. Please try again."
				}
				s.renderProfile(w, r, user, address, lookupErr, false)
				return
			}
			if _, err := s.store.UpdateUserLocation(user.ID, result.FederalRidingID, result.ProvincialRidingID); err != nil {
				http.Error(w, "failed to save profile", http.StatusInternalServerError)
				return
			}
			s.setLocalRidingCookies(w, result.FederalRidingID, result.ProvincialRidingID)
			http.Redirect(w, r, "/profile?updated=1", http.StatusSeeOther)
			return
		}

		if federalRiding != "" || provincialRiding != "" {
			// Manual riding selection path.
			if _, err := s.store.UpdateUserLocation(user.ID, federalRiding, provincialRiding); err != nil {
				http.Error(w, "failed to save profile", http.StatusInternalServerError)
				return
			}
			s.setLocalRidingCookies(w, federalRiding, provincialRiding)
			http.Redirect(w, r, "/profile?updated=1", http.StatusSeeOther)
			return
		}

		// Empty submission — clear saved location.
		if _, err := s.store.UpdateUserLocation(user.ID, "", ""); err != nil {
			http.Error(w, "failed to clear address", http.StatusInternalServerError)
			return
		}
		s.setLocalRidingCookies(w, "", "")
		http.Redirect(w, r, "/profile?updated=1", http.StatusSeeOther)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderProfile(w http.ResponseWriter, r *http.Request, user store.UserRow, address, lookupErr string, updated bool) {
	address = strings.TrimSpace(address)
	var result riding.LookupResult
	if address != "" {
		lookup, err := s.riding.Lookup(r.Context(), address)
		if err == nil {
			result = lookup
		} else if lookupErr == "" {
			log.Printf("renderProfile lookup failed for %q: %v", address, err)
			result = fallbackLookupResult(user.FederalRidingID, user.ProvincialRidingID, s.store)
		}
	} else {
		result = fallbackLookupResult(user.FederalRidingID, user.ProvincialRidingID, s.store)
	}

	cats, _ := s.store.GetUserCategoryPreferences(user.ID)
	federalRidings, _ := s.store.ListDistinctRidingsByLevel("federal")
	provincialRidings, _ := s.store.ListDistinctRidingsByLevel("provincial")

	_ = templates.ProfilePage(
		s.parliamentStatus(),
		user,
		address,
		result.Representatives,
		result.FederalRepresentative,
		result.ProvincialRepresentative,
		lookupErr,
		updated,
		s.riding.PlacesAPIKey(),
		cats,
		federalRidings,
		provincialRidings,
	).Render(r.Context(), w)
}
