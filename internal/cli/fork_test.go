package cli

import "testing"

func TestParseRef(t *testing.T) {
	cases := []struct {
		in      string
		user    string
		slug    string
		wantErr bool
	}{
		{"@alice/thing", "alice", "thing", false},
		{"alice/thing", "alice", "thing", false},
		{"alice", "", "", true},
		{"@/thing", "", "", true},
		{"", "", "", true},
		{"alice/", "", "", true},
	}
	for _, tc := range cases {
		u, s, err := parseRef(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseRef(%q) err = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			continue
		}
		if u != tc.user || s != tc.slug {
			t.Errorf("parseRef(%q) = (%q, %q), want (%q, %q)", tc.in, u, s, tc.user, tc.slug)
		}
	}
}

func TestExtensionForMapsFileType(t *testing.T) {
	if ext := extensionFor("jsx"); ext != "jsx" {
		t.Errorf("jsx -> %q, want jsx", ext)
	}
	if ext := extensionFor("html"); ext != "html" {
		t.Errorf("html -> %q, want html", ext)
	}
	if ext := extensionFor("weird"); ext != "html" {
		t.Errorf("unknown -> %q, want html fallback", ext)
	}
}
