package regexputil_test

import (
	"testing"

	"github.com/G-Research/geras/pkg/regexputil"
)

func TestList(t *testing.T) {
	for _, r := range []struct {
		re     string
		err    bool
		ok     bool
		expect []string
	}{
		// Normal cases we expect to handle
		{"", false, true, []string{""}},
		{"xx|yy", false, true, []string{"xx", "yy"}},
		{"xx|yy", false, true, []string{"xx", "yy"}},
		{"(xx|yy)", false, true, []string{"xx", "yy"}},
		{"(?:xx|yy)", false, true, []string{"xx", "yy"}},
		{"^(?:xx|yy)$", false, true, []string{"xx", "yy"}},
		{"^(xx|yy)$", false, true, []string{"xx", "yy"}},
		{"^(xx|yy)", false, true, []string{"xx", "yy"}},
		{"^(xx|yy)", false, true, []string{"xx", "yy"}},
		// Handled as CharClasses instead of Literals, so test explicitly.
		{"x|y|z", false, true, []string{"x", "y", "z"}},
		{"([ab])", false, true, []string{"a", "b"}},
		{"[a-f]", false, true, []string{"a", "b", "c", "d", "e", "f"}},
		{"long1|long2", false, true, []string{"long1", "long2"}},
		{"long[1-9]", false, true, []string{"long1", "long2", "long3", "long4", "long5", "long6", "long7", "long8", "long9"}},
		// Alternates are the same, simplified to one.
		{"samething|samething", false, true, []string{"samething"}},

		// We don't handle some aspect
		{"(^xx|^yy)", false, false, nil},         // Would be easy, but who writes regexps like that anyway.
		{"^$", false, false, nil},                // Better BeginText/EndText handling could fix this too, probably not worth it.
		{"longx[1-3]|longy1", false, false, nil}, // charclasses and alternatives, potentially doable...
		{"long[1-3]x", false, false, nil},        // charclasses and literals around it, same
		{"^(?i:xx|yy)$", false, false, nil},
		{"(xx|yy.)", false, false, nil},
		{"(xx|yy.*)", false, false, nil},
		{"(xx|yy).", false, false, nil},
		{"(xx|yy).*", false, false, nil},
		{"(xx|yy)*", false, false, nil},
		{".", false, false, nil},
	} {
		p, err := regexputil.Parse(r.re)
		if err != nil {
			if !r.err {
				t.Errorf("%q: got err, want !err", r.re)
			}
			continue
		}
		if r.err {
			t.Errorf("%q: got !err, want err", r.re)
		}
		l, ok := p.List()
		if ok != r.ok {
			t.Errorf("%q: got %v, want %v", r.re, ok, r.ok)
		}
		if len(l) != len(r.expect) {
			t.Errorf("%q: got %d items, want %d", r.re, len(l), len(r.expect))
		}
		for i, item := range r.expect {
			if l[i] != item {
				t.Errorf("%q: got l[%d] = %v, want %v", r.re, i, l[i], item)
			}
		}
	}
}

func TestWildcard(t *testing.T) {
	for _, r := range []struct {
		re     string
		err    bool
		ok     bool
		expect string
	}{
		// Normal cases we expect to handle
		{"", false, false, ""},
		{".*", false, true, "*"},
		{"test.*", false, true, "test*"},
		{".*test.*", false, true, "*test*"},
		{".*test", false, true, "*test"},
		{".*test.*test", false, true, "*test*test"},

		// Multiple stars, questionable but handled...
		{".*.*test", false, true, "**test"},
		{".*.*test.*.*.*", false, true, "**test***"},

		// Unhandled
		{".+", false, false, ""},

		// Errors
		{".*test.**test", true, false, ""},
		{"*", true, false, ""},
	} {
		p, err := regexputil.Parse(r.re)
		if err != nil {
			if !r.err {
				t.Errorf("%q: got err, want !err", r.re)
			}
			continue
		}
		if r.err {
			t.Errorf("%q: got !err, want err", r.re)
		}
		l, ok := p.Wildcard()
		if ok != r.ok {
			t.Errorf("%q: got %v, want %v", r.re, ok, r.ok)
		}
		if l != r.expect {
			t.Errorf("%q: got %q, want %q", r.re, l, r.expect)
		}
	}
}
