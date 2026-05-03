// Package server wires HTTP routes to the store and renders templ templates.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/auth"
	"github.com/philspins/opendocket/internal/opennorth"
	"github.com/philspins/opendocket/internal/riding"
	"github.com/philspins/opendocket/internal/scraper"
	"github.com/philspins/opendocket/internal/store"
	"github.com/philspins/opendocket/internal/templates"
	"github.com/philspins/opendocket/internal/visitor"
)

// localRidingContextToken indicates local riding context exists without exposing
// specific riding identifiers to the template.
const localRidingContextToken = "local-riding"

// Server holds application dependencies.
type Server struct {
	store   *store.Store
	mux     *http.ServeMux
	auth    *auth.Service
	riding  *riding.Service
	baseURL string
	// baseHost is the hostname extracted from baseURL at construction time.
	// It is used for the HTTP→HTTPS redirect to prevent host-header injection.
	// Empty when baseURL cannot be parsed or contains no host.
	baseHost                   string
	trustProxy                 bool
	billInteractionRateLimit   int
	billInteractionRateLimiter *simpleRateLimiter
}

// New creates a Server and registers all routes.
func New(st *store.Store) *Server {
	baseURL := strings.TrimRight(os.Getenv("OAUTH_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	googleMapsKey := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))

	parsed, err := url.Parse(baseURL)
	baseHost := ""
	if err != nil {
		log.Printf("warning: could not parse OAUTH_BASE_URL %q: %v; HTTP→HTTPS redirect disabled", baseURL, err)
	} else if parsed != nil {
		baseHost = parsed.Host
	}

	s := &Server{
		store:                      st,
		mux:                        http.NewServeMux(),
		auth:                       auth.New(st, baseURL),
		riding:                     riding.New(st, googleMapsKey),
		baseURL:                    baseURL,
		baseHost:                   baseHost,
		trustProxy:                 strings.ToLower(strings.TrimSpace(os.Getenv("TRUST_PROXY"))) == "true",
		billInteractionRateLimit:   envInt("BILL_INTERACTION_RATE_LIMIT_PER_MINUTE", 10),
		billInteractionRateLimiter: newSimpleRateLimiter(),
	}

	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /", s.handleHome)
	s.mux.HandleFunc("GET /bills", s.handleBills)
	s.mux.HandleFunc("GET /bills/{id}", s.handleBillDetail)
	s.mux.HandleFunc("GET /votes", s.handleVotes)
	s.mux.HandleFunc("GET /members", s.handleMembers)
	s.mux.HandleFunc("GET /members/{id}", s.handleMemberProfile)
	s.mux.HandleFunc("GET /compare", s.handleCompare)
	s.mux.HandleFunc("GET /profile", s.handleProfile)
	s.mux.HandleFunc("POST /profile", s.handleProfile)
	s.mux.HandleFunc("GET /privacy", s.handlePrivacy)
	s.mux.HandleFunc("GET /tos", s.handleTerms)
	s.mux.HandleFunc("GET /delete-data", s.handleDeleteDataPage)
	s.mux.HandleFunc("POST /delete-data", s.handleDeleteDataCallback)
	s.mux.HandleFunc("GET /riding", s.handleRiding)
	s.mux.HandleFunc("GET /feedback", s.handleFeedback)
	s.mux.HandleFunc("POST /feedback", s.handleFeedback)
	s.mux.HandleFunc("POST /api/follow", s.handleFollow)
	s.mux.HandleFunc("POST /api/react", s.handleReact)
	s.mux.HandleFunc("POST /api/subscribe-bill", s.handleSubscribeBill)
	s.mux.HandleFunc("POST /api/log-submission", s.handleLogSubmission)
	s.mux.HandleFunc("POST /profile/interests", s.handleProfileInterests)
	s.auth.RegisterRoutes(s.mux)

	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// When running behind a trusted reverse proxy in HTTPS mode, redirect
	// plain-HTTP requests to HTTPS. The /health path is exempt so that ALB
	// health checks always succeed regardless of protocol.
	// s.baseHost is derived from OAUTH_BASE_URL at startup; if it is empty the
	// redirect is skipped to avoid an open-redirect via a spoofed Host header.
	if s.trustProxy && s.baseHost != "" && strings.HasPrefix(s.baseURL, "https://") &&
		strings.TrimSuffix(r.URL.Path, "/") != "/health" {
		if strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))) == "http" {
			http.Redirect(w, r, "https://"+s.baseHost+r.RequestURI, http.StatusMovedPermanently)
			return
		}
	}
	s.applySecurityHeaders(w, r)
	s.mux.ServeHTTP(w, r)
}

func (s *Server) applySecurityHeaders(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	h.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"frame-src 'self' https://www.google.com/recaptcha/ https://accounts.google.com https://www.facebook.com",
		"object-src 'none'",
		"img-src 'self' data: https:",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://accounts.google.com",
		"font-src 'self' https://fonts.gstatic.com",
		"script-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com https://accounts.google.com https://connect.facebook.net https://maps.googleapis.com https://maps.gstatic.com https://www.google.com/recaptcha/ https://www.gstatic.com/recaptcha/",
		"connect-src 'self' https://graph.facebook.com https://www.googleapis.com https://oauth2.googleapis.com https://maps.googleapis.com https://www.google.com/recaptcha/",
	}, "; "))

	isHTTPS := r.TLS != nil
	if !isHTTPS && s.trustProxy {
		isHTTPS = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))) == "https"
	}
	if isHTTPS {
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

func (s *Server) parliamentStatus() store.ParliamentStatus {
	ps, _ := s.store.GetParliamentStatus(scraper.CurrentParliament, scraper.CurrentSession)
	return ps
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ps := s.parliamentStatus()
	var (
		savedAddress    string
		federalRep      opennorth.Representative
		provincialRep   opennorth.Representative
		federalVotes    []store.VoteRow
		provincialVotes []store.VoteRow
		federalStatus   = parliamentStatusText(ps.Status)
		provStatus      = parliamentStatusText("status_unavailable")
	)
	v := visitor.FromRequest(r, s.auth)
	result := fallbackLookupResult(v.FederalRidingID, v.ProvincialRidingID, s.store)
	if hasLocalRidingContext(result) {
		savedAddress = localRidingContextToken
	}
	federalRep = result.FederalRepresentative
	provincialRep = result.ProvincialRepresentative
	if memberID := s.resolveRepresentativeMemberID(federalRep, true); memberID != "" {
		votes, err := s.store.GetMemberVotes(memberID, 100)
		if err != nil {
			log.Printf("home: failed loading federal member votes for %q: %v", memberID, err)
		} else {
			federalVotes = recentBillVotes(votes, 5)
		}
	}
	provincialProvince := ""
	if memberID := s.resolveRepresentativeMemberID(provincialRep, false); memberID != "" {
		votes, err := s.store.GetMemberVotes(memberID, 100)
		if err != nil {
			log.Printf("home: failed loading provincial member votes for %q: %v", memberID, err)
		} else {
			provincialVotes = recentBillVotes(votes, 5)
		}
		if member, err := s.store.GetMember(memberID); err == nil {
			provincialProvince = member.Province
		}
	}
	if status, err := s.store.GetCombinedJurisdictionStatus("federal-commons", "federal-senate"); err == nil {
		if resolved := strings.TrimSpace(parliamentStatusText(status)); resolved != "" && resolved != "status unavailable" {
			federalStatus = resolved
		}
	}
	if key := provinceJurisdictionKey(provincialProvince); key != "" {
		if status, err := s.store.GetJurisdictionStatus(key); err == nil {
			if resolved := strings.TrimSpace(parliamentStatusText(status)); resolved != "" && resolved != "status unavailable" {
				provStatus = resolved
			}
		}
	}
	_, isLoggedIn := s.auth.SessionUser(r)
	_ = templates.Home(ps, provincialVotes, federalVotes, savedAddress, federalRep, provincialRep, provStatus, federalStatus, isLoggedIn).Render(r.Context(), w)
}

func provinceJurisdictionKey(province string) string {
	p := strings.ToUpper(strings.TrimSpace(province))
	switch p {
	case "AB", "ALBERTA":
		return "provincial-AB"
	case "BC", "BRITISH COLUMBIA":
		return "provincial-BC"
	case "MB", "MANITOBA":
		return "provincial-MB"
	case "NB", "NEW BRUNSWICK":
		return "provincial-NB"
	case "NL", "NEWFOUNDLAND AND LABRADOR":
		return "provincial-NL"
	case "NS", "NOVA SCOTIA":
		return "provincial-NS"
	case "ON", "ONTARIO":
		return "provincial-ON"
	case "PE", "PEI", "PRINCE EDWARD ISLAND":
		return "provincial-PE"
	case "QC", "QUEBEC", "QUÉBEC":
		return "provincial-QC"
	case "SK", "SASKATCHEWAN":
		return "provincial-SK"
	case "YT", "YUKON":
		return "provincial-YT"
	case "NT", "NORTHWEST TERRITORIES":
		return "provincial-NT"
	case "NU", "NUNAVUT":
		return "provincial-NU"
	default:
		return ""
	}
}

func (s *Server) resolveRepresentativeMemberID(rep opennorth.Representative, federal bool) string {
	if id := strings.TrimSpace(rep.LocalMemberID); id != "" {
		return id
	}
	ridingName := strings.TrimSpace(rep.DistrictName)
	if ridingName == "" {
		return ""
	}
	members, err := s.store.GetMembersByRiding(ridingName)
	if err != nil || len(members) == 0 {
		return ""
	}

	targetLevel := "provincial"
	if federal {
		targetLevel = "federal"
	}
	name := strings.TrimSpace(rep.Name)
	fallback := ""
	anyLevelNameMatch := ""
	for _, member := range members {
		nameMatch := name != "" && strings.EqualFold(strings.TrimSpace(member.Name), name)
		if !strings.EqualFold(strings.TrimSpace(member.GovernmentLevel), targetLevel) {
			if anyLevelNameMatch == "" && nameMatch {
				anyLevelNameMatch = member.ID
			}
			continue
		}
		if fallback == "" {
			fallback = member.ID
		}
		if nameMatch {
			return member.ID
		}
	}
	if fallback != "" {
		return fallback
	}
	if anyLevelNameMatch != "" {
		return anyLevelNameMatch
	}
	return ""
}

func recentBillVotes(votes []store.VoteRow, limit int) []store.VoteRow {
	if limit <= 0 {
		limit = 5
	}
	out := make([]store.VoteRow, 0, limit)
	seen := make(map[string]struct{}, len(votes))
	// First pass: prefer votes linked to bills.
	for _, vote := range votes {
		billID := strings.TrimSpace(vote.BillID)
		if billID == "" {
			continue
		}
		if _, ok := seen[billID]; ok {
			continue
		}
		seen[billID] = struct{}{}
		out = append(out, vote)
		if len(out) == limit {
			return out
		}
	}
	// Second pass: fill remaining slots from non-bill votes (e.g. provincial divisions).
	for _, vote := range votes {
		if strings.TrimSpace(vote.BillID) != "" {
			continue
		}
		key := strings.TrimSpace(vote.DivisionID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, vote)
		if len(out) == limit {
			return out
		}
	}
	return out
}

func parliamentStatusText(status string) string {
	switch strings.TrimSpace(status) {
	case "in_session":
		return "in session"
	case "on_break":
		return "on break"
	default:
		return "status unavailable"
	}
}

func (s *Server) handleBills(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	sortParam := q.Get("sort")
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage != 5 && perPage != 10 && perPage != 25 && perPage != 50 {
		perPage = 10
	}
	f := store.BillFilter{
		Search:   q.Get("q"),
		Stage:    q.Get("stage"),
		Category: q.Get("category"),
		Chamber:  q.Get("chamber"),
		Level:    q.Get("level"),
		Sort:     sortParam,
		Page:     page,
		PerPage:  perPage,
	}

	// For personalized "auto" sort, load the user's preferences and subscriptions.
	var subscribedIDs []string
	if sortParam == "auto" {
		if user, ok := s.auth.SessionUser(r); ok {
			cats, _ := s.store.GetUserCategoryPreferences(user.ID)
			subs, _ := s.store.GetUserBillSubscriptions(user.ID)
			f.PreferredCategories = cats
			f.SubscribedBillIDs = subs
			subscribedIDs = subs
		}
	}

	ps := s.parliamentStatus()
	bills, total, err := s.store.ListBills(f)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	provinces, _ := s.store.ListDistinctProvinces()
	_ = templates.BillsFeed(ps, bills, total, f, subscribedIDs, provinces).Render(r.Context(), w)
}

func (s *Server) handleBillDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ps := s.parliamentStatus()
	user, isAuthenticated := s.auth.SessionUser(r)
	bill, err := s.store.GetBill(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	stages, _ := s.store.GetBillStages(id)
	divs, _ := s.store.GetBillDivisions(id)
	reactions, _ := s.store.GetBillReactionCounts(id)
	var isSubscribed bool
	if isAuthenticated {
		isSubscribed, _ = s.store.IsUserSubscribedToBill(user.ID, id)
	}
	_ = templates.BillDetail(ps, bill, stages, divs, reactions, isAuthenticated, isSubscribed).Render(r.Context(), w)
}

func (s *Server) handleVotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage != 5 && perPage != 10 && perPage != 25 && perPage != 50 {
		perPage = 10
	}
	ps := s.parliamentStatus()
	divs, total, err := s.store.ListDivisions(page, perPage)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_ = templates.VotesFeed(ps, divs, total, page, perPage).Render(r.Context(), w)
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ps := s.parliamentStatus()
	members, err := s.store.ListMembers(q.Get("q"), q.Get("party"), q.Get("province"), q.Get("riding"), q.Get("level"))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	parties, _ := s.store.ListDistinctParties()
	provinces, _ := s.store.ListDistinctProvinces()
	ridings, _ := s.store.ListDistinctRidings()
	_ = templates.MembersDirectory(ps, members, q.Get("q"), q.Get("party"), q.Get("province"), q.Get("riding"), q.Get("level"), parties, provinces, ridings).Render(r.Context(), w)
}

func (s *Server) handleMemberProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ps := s.parliamentStatus()
	member, err := s.store.GetMember(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	votes, _ := s.store.GetMemberVotes(id, 500)
	stats, _ := s.store.GetMemberStats(id)
	catScores, _ := s.store.GetMemberCategoryScores(id)
	_ = templates.MemberProfile(ps, member, votes, stats, catScores).Render(r.Context(), w)
}

func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ps := s.parliamentStatus()
	var m1, m2 store.MemberRow
	var sharedVotes []store.SharedVoteRow
	var overlap, total int
	level := q.Get("level")
	if level == "" {
		level = "federal"
	}

	provincialMembers, err := s.store.ListMembers("", "", "", "", "provincial")
	if err != nil {
		log.Printf("handleCompare: list provincial members: %v", err)
	}
	provinces := uniqueSortedMemberFields(provincialMembers, func(m store.MemberRow) string { return m.Province })
	province := q.Get("province")
	if level != "provincial" || !isValidSelection(provinces, province) {
		province = ""
	}

	partyBaseMembers, err := s.store.ListMembers("", "", province, "", level)
	if err != nil {
		log.Printf("handleCompare: list party base members: %v", err)
	}
	parties := uniqueSortedMemberFields(partyBaseMembers, func(m store.MemberRow) string { return m.Party })
	party := q.Get("party")
	if !isValidSelection(parties, party) {
		party = ""
	}

	members, err := s.store.ListMembers("", party, province, "", level)
	if err != nil {
		log.Printf("handleCompare: list filtered members: %v", err)
	}
	memberIDs := make(map[string]struct{}, len(members))
	for _, m := range members {
		memberIDs[m.ID] = struct{}{}
	}

	idA, idB := q.Get("a"), q.Get("b")
	if _, ok := memberIDs[idA]; idA != "" && ok {
		m1, err = s.store.GetMember(idA)
		if err != nil {
			log.Printf("handleCompare: get member a %q: %v", idA, err)
		}
	}
	if _, ok := memberIDs[idB]; idB != "" && ok {
		m2, err = s.store.GetMember(idB)
		if err != nil {
			log.Printf("handleCompare: get member b %q: %v", idB, err)
		}
	}
	if m1.ID != "" && m2.ID != "" {
		overlap, total, err = s.store.CompareMemberVotes(m1.ID, m2.ID)
		if err != nil {
			log.Printf("handleCompare: compare votes %q vs %q: %v", m1.ID, m2.ID, err)
		}
		sharedVotes, err = s.store.GetSharedMemberVotes(m1.ID, m2.ID, 100)
		if err != nil {
			log.Printf("handleCompare: shared votes %q vs %q: %v", m1.ID, m2.ID, err)
		}
	}
	if err := templates.CompareMPs(ps, members, m1, m2, level, province, party, provinces, parties, overlap, total, sharedVotes).Render(r.Context(), w); err != nil {
		log.Printf("handleCompare: render: %v", err)
	}
}

func uniqueSortedMemberFields(members []store.MemberRow, field func(store.MemberRow) string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, m := range members {
		value := strings.TrimSpace(field(m))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isValidSelection(values []string, needle string) bool {
	if needle == "" {
		return true
	}
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}

func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	u, ok := s.auth.RequireVerifiedSessionUser(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	memberID := r.FormValue("member_id")
	if strings.TrimSpace(memberID) == "" {
		http.Error(w, "member_id required", http.StatusBadRequest)
		return
	}
	if err := s.store.FollowMember(u.Email, memberID); err != nil {
		http.Error(w, "failed to follow", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/members/"+memberID, http.StatusSeeOther)
}

func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	u, ok := s.auth.RequireVerifiedSessionUser(w, r)
	if !ok {
		return
	}
	if s.billInteractionRateLimit > 0 && s.billInteractionRateLimiter != nil &&
		!s.billInteractionRateLimiter.allow(billInteractionRateKey(u.Email), s.billInteractionRateLimit, time.Minute, time.Now().UTC()) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	billID := r.FormValue("bill_id")
	reaction := r.FormValue("reaction")
	note := r.FormValue("note")
	if strings.TrimSpace(billID) == "" {
		http.Error(w, "bill_id required", http.StatusBadRequest)
		return
	}
	if err := s.store.ReactToBill(u.Email, billID, reaction, note); err != nil {
		http.Error(w, "failed to save reaction", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/bills/"+billID, http.StatusSeeOther)
}

func (s *Server) handleLogSubmission(w http.ResponseWriter, r *http.Request) {
	u, ok := s.auth.RequireVerifiedSessionUser(w, r)
	if !ok {
		return
	}
	var payload struct {
		MemberID string `json:"member_id"`
		Subject  string `json:"subject"`
		Body     string `json:"body"`
		Category string `json:"category"`
	}

	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		payload.MemberID = r.FormValue("member_id")
		payload.Subject = r.FormValue("subject")
		payload.Body = r.FormValue("body")
		payload.Category = r.FormValue("category")
	}

	if strings.TrimSpace(payload.MemberID) == "" {
		http.Error(w, "member_id required", http.StatusBadRequest)
		return
	}
	if err := s.store.LogPolicySubmission(
		u.Email,
		payload.MemberID,
		payload.Subject,
		payload.Body,
		payload.Category,
	); err != nil {
		http.Error(w, "failed to log submission", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleSubscribeBill(w http.ResponseWriter, r *http.Request) {
	u, ok := s.auth.RequireVerifiedSessionUser(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	billID := strings.TrimSpace(r.FormValue("bill_id"))
	if billID == "" {
		http.Error(w, "bill_id required", http.StatusBadRequest)
		return
	}
	if _, err := s.store.ToggleBillSubscription(u.ID, billID); err != nil {
		http.Error(w, "failed to toggle subscription", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/bills/"+billID, http.StatusSeeOther)
}

func (s *Server) handleProfileInterests(w http.ResponseWriter, r *http.Request) {
	user, ok := s.auth.SessionUser(r)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	categories := r.Form["categories"]
	if err := s.store.SaveUserCategoryPreferences(user.ID, categories); err != nil {
		http.Error(w, "failed to save preferences", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/profile?updated=1", http.StatusSeeOther)
}

func billInteractionRateKey(email string) string {
	return "bill:react:" + strings.ToLower(strings.TrimSpace(email))
}
