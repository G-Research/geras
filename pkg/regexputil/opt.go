// Package regexputil implements some introspection on parsed regexps.
package regexputil

import (
	"regexp/syntax"
)

// Regexp represents a parsed regexp. Use Parse to make one.
type Regexp struct {
	pt *syntax.Regexp
}

// Parse takes a regexp in Perl syntax (as implemented by regexp/syntax) and
// returns a Regexp, for introspecting the regexp.
func Parse(regexp string) (Regexp, error) {
	pt, err := syntax.Parse(regexp, syntax.Perl)
	if err != nil {
		return Regexp{}, err
	}
	pt = pt.Simplify()
	return Regexp{pt: pt}, nil
}

// List returns a list of fixed matches if the regexp only matches a fixed set
// of alternate strings.
func (r Regexp) List() ([]string, bool) {
	potential := r.recurse([]*syntax.Regexp{r.pt}, 0, 0)
	if len(potential) == 0 {
		return nil, false
	}
	items := make([]string, len(potential))
	for i, p := range potential {
		items[i] = string(p)
	}
	return items, true
}

func (r Regexp) recurse(p []*syntax.Regexp, parentOp syntax.Op, level int) [][]rune {
	var potential [][]rune
	// Concat, Capture, Alternate is the most we handle
	if level > 3 {
		return nil
	}
	for i, s := range p {
		// Ignore (?i), etc.
		if (s.Flags & (syntax.FoldCase | syntax.DotNL)) != 0 {
			return nil
		}
		switch s.Op {
		case syntax.OpConcat:
			if len(potential) != 0 {
				return nil
			}
			potential = r.recurse(s.Sub, s.Op, level+1)
		case syntax.OpCapture:
			if len(potential) != 0 {
				return nil
			}
			potential = r.recurse(s.Sub, s.Op, level+1)
		case syntax.OpAlternate:
			if len(potential) != 0 {
				return nil
			}
			potential = r.recurse(s.Sub, s.Op, level+1)
		case syntax.OpCharClass:
			if len(potential) > 0 && parentOp != syntax.OpAlternate {
				return nil
			}
			// Rune is a list of pairs of character ranges in this case, we have to expand
			for i := 0; i < len(s.Rune); i += 2 {
				start, end := s.Rune[i], s.Rune[i+1]
				for r := start; r <= end; r++ {
					potential = append(potential, []rune{r})
				}
			}
		case syntax.OpLiteral:
			if len(potential) > 0 && parentOp != syntax.OpAlternate {
				return nil
			}
			potential = append(potential, s.Rune)
		case syntax.OpEmptyMatch:
			if len(potential) > 0 && parentOp != syntax.OpAlternate {
				return nil
			}
			potential = append(potential, []rune{})
		// We only handle full matches on single lines as that's what Prometheus uses.
		// ^ and $ are therefore meaningless, but people do use them, so ignore if in the correct place.
		case syntax.OpBeginText:
			if i != 0 {
				// invalid, skip
				return nil
			}
		case syntax.OpEndText:
			if i != len(p)-1 {
				// invalid, skip
				return nil
			}
		default:
			return nil // unknown op, can't do anything
		}
	}
	return potential
}
