package templates

import (
	"strings"
	"testing"
)

// ── modeTitle / modeHeading / modeVerb / googleWidgetText ────────────────────

func TestModeTitle(t *testing.T) {
	tests := []struct{ mode, want string }{
		{"signup", "Sign Up"},
		{"Signup", "Sign Up"},
		{"SIGNUP", "Sign Up"},
		{"login", "Login"},
		{"", "Login"},
	}
	for _, tt := range tests {
		if got := modeTitle(tt.mode); got != tt.want {
			t.Errorf("modeTitle(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestModeHeading(t *testing.T) {
	if got := modeHeading("signup"); got != "Create Your Account" {
		t.Errorf("modeHeading(signup) = %q, want %q", got, "Create Your Account")
	}
	if got := modeHeading("login"); got != "Welcome Back" {
		t.Errorf("modeHeading(login) = %q, want %q", got, "Welcome Back")
	}
}

func TestModeVerb(t *testing.T) {
	if got := modeVerb("signup"); got != "create" {
		t.Errorf("modeVerb(signup) = %q, want create", got)
	}
	if got := modeVerb("login"); got != "access" {
		t.Errorf("modeVerb(login) = %q, want access", got)
	}
}

func TestGoogleWidgetText(t *testing.T) {
	if got := googleWidgetText("signup"); got != "signup_with" {
		t.Errorf("googleWidgetText(signup) = %q, want signup_with", got)
	}
	if got := googleWidgetText("login"); got != "signin_with" {
		t.Errorf("googleWidgetText(login) = %q, want signin_with", got)
	}
}

// ── signupAccountAccessClass ──────────────────────────────────────────────────

func TestSignupAccountAccessClass(t *testing.T) {
	// With recaptcha enabled (signup + key) → dimmed CSS classes
	got := signupAccountAccessClass("signup", "site-key-123")
	if !strings.Contains(got, "opacity-50") {
		t.Errorf("signupAccountAccessClass(signup, key) = %q, expected opacity-50 class", got)
	}

	// Without recaptcha (login or empty key) → normal class
	got2 := signupAccountAccessClass("login", "site-key-123")
	if strings.Contains(got2, "opacity-50") {
		t.Errorf("signupAccountAccessClass(login, key) = %q, should not contain opacity-50", got2)
	}
}

// ── isSafeRecaptchaSiteKey ────────────────────────────────────────────────────

func TestIsSafeRecaptchaSiteKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"6LcABCDEFG-_1234", true},
		{"validKey123", true},
		{"", false},
		{"key with space", false},
		{"key<script>", false},
		{"key&param=x", false},
	}
	for _, tt := range tests {
		if got := isSafeRecaptchaSiteKey(tt.key); got != tt.want {
			t.Errorf("isSafeRecaptchaSiteKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

// ── recaptchaScriptSrc ────────────────────────────────────────────────────────

func TestRecaptchaScriptSrc_SafeKey(t *testing.T) {
	got := recaptchaScriptSrc("MySiteKey123")
	if !strings.HasPrefix(got, "https://www.google.com/recaptcha/api.js?") {
		t.Errorf("recaptchaScriptSrc(safe) = %q, want URL with query string", got)
	}
	if !strings.Contains(got, "render=MySiteKey123") {
		t.Errorf("recaptchaScriptSrc(safe) = %q, should contain render= param", got)
	}
}

func TestRecaptchaScriptSrc_UnsafeKey(t *testing.T) {
	got := recaptchaScriptSrc("<script>bad</script>")
	// Should return base URL without query params (unsafe key rejected)
	if got != "https://www.google.com/recaptcha/api.js" {
		t.Errorf("recaptchaScriptSrc(unsafe) = %q, want base URL", got)
	}
}

func TestRecaptchaScriptSrc_EmptyKey(t *testing.T) {
	got := recaptchaScriptSrc("")
	if got != "https://www.google.com/recaptcha/api.js" {
		t.Errorf("recaptchaScriptSrc(empty) = %q, want base URL", got)
	}
}

// ── signupRecaptchaEnabled ────────────────────────────────────────────────────

func TestSignupRecaptchaEnabled(t *testing.T) {
	tests := []struct {
		name string
		mode string
		key  string
		want bool
	}{
		{name: "signup with key", mode: "signup", key: "site-key", want: true},
		{name: "signup case-insensitive", mode: "SignUp", key: "site-key", want: true},
		{name: "signup without key", mode: "signup", key: "", want: false},
		{name: "login with key", mode: "login", key: "site-key", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := signupRecaptchaEnabled(tt.mode, tt.key); got != tt.want {
				t.Fatalf("signupRecaptchaEnabled(%q, %q) = %v, want %v", tt.mode, tt.key, got, tt.want)
			}
		})
	}
}
