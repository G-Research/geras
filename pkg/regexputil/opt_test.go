package regexputil_test

import (
  "testing"

	"github.com/G-Research/geras/pkg/regexputil"
)

func TestList(t *testing.T) {
	for _, r := range []struct {
		re string
		err bool
		ok bool
		expect []string
	} {
		// Normal cases we expect to handle
		{ "", false, true, []string{""} },
		{ "xx|yy", false, true, []string{"xx", "yy"} },
		{ "xx|yy", false, true, []string{"xx", "yy"} },
		{ "(xx|yy)", false, true, []string{"xx", "yy"} },
		{ "(?:xx|yy)", false, true, []string{"xx", "yy"} },
		{ "^(?:xx|yy)$", false, true, []string{"xx", "yy"} },
		{ "^(xx|yy)$", false, true, []string{"xx", "yy"} },
		{ "^(xx|yy)", false, true, []string{"xx", "yy"} },
		{ "^(xx|yy)", false, true, []string{"xx", "yy"} },

		// We don't handle some aspect
		{ "(^xx|^yy)", false, false, nil }, // Would be easy, but who writes regexps like that anyway.
		{ "^$", false, false, nil }, // Better BeginText/EndText handling could fix this too, probably not worth it.
		{ "^(?i:xx|yy)$", false, false, nil },
		{ "(xx|yy.)", false, false, nil },
		{ "(xx|yy.*)", false, false, nil },
		{ "(xx|yy).", false, false, nil },
		{ "(xx|yy).*", false, false, nil },
		{ "(xx|yy)*", false, false, nil },
		{ "[ab]", false, false, nil },
		{ "(x|y)", false, false, nil }, // Simplify makes into a charclass for us...
		{ "x|y|z", false, false, nil }, // Same
		{ ".", false, false, nil },
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
	}
}
