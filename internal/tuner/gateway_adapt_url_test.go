package tuner

import "testing"

func TestStreamURLsSemanticallyEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"http://a/x", "http://a/x", true},
		{"http://a/x/", "http://a/x", true},
		{"HTTP://a/x/", "http://a/x", true},
		{"https://a/x", "http://a/x", false},
		{"http://a:80/x", "http://a/x", true},
		{"https://a:443/", "https://a/", true},
		{"http://a/x?p=1", "http://a/x", false},
		{"http://a/x?p=1", "http://a/x?p=1", true},
		{"http://b/x", "http://a/x", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.a+"|"+tc.b, func(t *testing.T) {
			if got := streamURLsSemanticallyEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
