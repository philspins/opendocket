package templates

import "testing"

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
