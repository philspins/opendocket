package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/philspins/opendocket/internal/store"
)

const verifyRecaptchaRateLimitKeyPrefix = "auth:verify-recaptcha:ip:"

func (s *Service) HandleRequestVerification(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAllowed("auth:request-verification:ip:"+s.clientIP(r), 10, time.Minute) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		if u, ok := s.SessionUser(r); ok {
			email = u.Email
		}
	}
	if email == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}
	if err := s.verifyRecaptcha(r.Context(), strings.TrimSpace(r.FormValue("g-recaptcha-response")), s.clientIP(r)); err != nil {
		log.Printf("recaptcha verification failed: %v", err)
		http.Error(w, "captcha verification failed", http.StatusBadRequest)
		return
	}
	if !s.rateLimitAllowed("auth:request-verification:email:"+strings.ToLower(email), 3, 10*time.Minute) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	_, code, err := s.store.CreateEmailVerification(email, 30*time.Minute)
	if err == nil && s.emailer != nil {
		verifyURL := s.baseURL + "/auth/verify"
		if sendErr := s.emailer.SendVerificationEmail(r.Context(), email, verifyURL, code); sendErr != nil {
			log.Printf("verification email send failed for %s: %v", email, sendErr)
		} else {
			log.Printf("verification email sent to %s", email)
		}
	} else if err == nil {
		log.Printf("verification requested for %s but SES is not configured", email)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Service) HandleVerifyRecaptcha(w http.ResponseWriter, r *http.Request) {
	// Keep this endpoint permissive enough for retries while still limiting abuse.
	if !s.rateLimitAllowed(verifyRecaptchaRateLimitKeyPrefix+s.clientIP(r), 20, time.Minute) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(r.FormValue("g-recaptcha-response"))
	if err := s.verifyRecaptcha(r.Context(), token, s.clientIP(r)); err != nil {
		log.Printf("signup recaptcha verification failed: %v", err)
		http.Error(w, "captcha verification failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (s *Service) verifyRecaptcha(ctx context.Context, token, clientIP string) error {
	secret := strings.TrimSpace(os.Getenv("RECAPTCHA_SECRET_KEY"))
	if secret == "" {
		return nil
	}
	if token == "" {
		return fmt.Errorf("missing recaptcha token")
	}
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if clientIP != "" {
		form.Set("remoteip", clientIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.google.com/recaptcha/api/siteverify", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("recaptcha API returned error status %d", resp.StatusCode)
	}
	var parsed struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		return fmt.Errorf("recaptcha verification failed")
	}
	return nil
}

func (s *Service) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimitAllowed("auth:verify:ip:"+s.clientIP(r), 20, time.Minute) {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	var token, email, code string
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var payload struct {
			Token string `json:"token"`
			Email string `json:"email"`
			Code  string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		token = strings.TrimSpace(payload.Token)
		email = strings.TrimSpace(payload.Email)
		code = strings.TrimSpace(payload.Code)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		token = strings.TrimSpace(r.FormValue("token"))
		email = strings.TrimSpace(r.FormValue("email"))
		code = strings.TrimSpace(r.FormValue("code"))
	}

	var (
		u   store.UserRow
		err error
	)
	if token != "" {
		u, err = s.store.VerifyEmailToken(token)
	} else {
		if email != "" && !s.rateLimitAllowed("auth:verify:email:"+strings.ToLower(email), 8, 10*time.Minute) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		u, err = s.store.VerifyEmailCode(email, code)
	}
	if err != nil {
		http.Error(w, "invalid verification credentials", http.StatusBadRequest)
		return
	}
	if err := s.setSessionCookie(w, u.ID); err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
