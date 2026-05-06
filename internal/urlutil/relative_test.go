package urlutil_test

import (
	"testing"

	"github.com/philspins/opendocket/internal/urlutil"
)

func TestResolveRelativeURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		href string
		want string
	}{
		{
			name: "absolute http href returned as-is",
			base: "https://example.com/foo",
			href: "http://other.com/bar",
			want: "http://other.com/bar",
		},
		{
			name: "absolute https href returned as-is",
			base: "https://example.com/foo",
			href: "https://cdn.example.com/img.png",
			want: "https://cdn.example.com/img.png",
		},
		{
			name: "root-relative path resolved against base host",
			base: "https://example.com/foo/bar",
			href: "/path/to/page",
			want: "https://example.com/path/to/page",
		},
		{
			name: "relative path resolved against base directory",
			base: "https://example.com/foo/bar",
			href: "baz",
			want: "https://example.com/foo/baz",
		},
		{
			name: "invalid base URL returns href unchanged",
			base: "://not-a-url",
			href: "/some/path",
			want: "/some/path",
		},
		{
			name: "invalid href returns href unchanged",
			base: "https://example.com/",
			href: "%zz",
			want: "%zz",
		},
		{
			name: "empty href resolves to base URL",
			base: "https://example.com/foo",
			href: "",
			want: "https://example.com/foo",
		},
		{
			name: "both empty returns empty string",
			base: "",
			href: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := urlutil.ResolveRelativeURL(tt.base, tt.href)
			if got != tt.want {
				t.Errorf("ResolveRelativeURL(%q, %q) = %q, want %q", tt.base, tt.href, got, tt.want)
			}
		})
	}
}
