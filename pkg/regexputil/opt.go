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
	potential := r.listRecurse([]*syntax.Regexp{r.pt}, 0, 0)
	if len(potential) == 0 {
		return nil, false
	}
	items := make([]string, len(potential))
	for i, p := range potential {
		items[i] = string(p)
	}
	return items, true
}

func (r Regexp) listRecurse(p []*syntax.Regexp, parentOp syntax.Op, level int) [][]rune {
	var potential [][]rune
	// Concat, Capture, Alternate, (a leaf op) is the most we handle
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
			potential = r.listRecurse(s.Sub, s.Op, level+1)
		case syntax.OpCapture:
			if len(potential) != 0 {
				return nil
			}
			potential = r.listRecurse(s.Sub, s.Op, level+1)
		case syntax.OpAlternate:
			if len(potential) != 0 {
				return nil
			}
			potential = r.listRecurse(s.Sub, s.Op, level+1)
		case syntax.OpCharClass:
			if len(potential) > 0 && (parentOp != syntax.OpAlternate && parentOp != syntax.OpConcat) {
				return nil
			}
			// Rune is a list of pairs of character ranges in this case, we have to expand
			runes := expandRunes(s.Rune)
			if parentOp == syntax.OpConcat {
				prefixes := potential
				potential = nil
				for i := range runes {
					for _, p := range prefixes {
						potential = append(potential, append(p, runes[i]...))
					}
				}
			} else {
				potential = runes
			}
		case syntax.OpLiteral:
			if len(potential) > 0 && parentOp != syntax.OpAlternate {
				return nil
			}
			potential = append(potential, s.Rune)
		case syntax.OpEmptyMatch:
			if len(potential) > 0 && (parentOp != syntax.OpAlternate && parentOp != syntax.OpConcat) {
				return nil
			}
			if parentOp != syntax.OpConcat {
				potential = append(potential, []rune{})
			}
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

func expandRunes(s []rune) [][]rune {
	var ret [][]rune
	for i := 0; i < len(s); i += 2 {
		start, end := s[i], s[i+1]
		for r := start; r <= end; r++ {
			ret = append(ret, []rune{r})
		}
	}
	return ret
}

// Wildcard attempts to convert the regexp into a wildcard form, that is, if it
// can be represented as a glob pattern using only "*", it will return "string*"
// and true. It is assumed the "*" glob character matches 0 or more characters
// only (i.e. is exactly identical to ".*"). Literal strings (i.e. not
// containing any "*") will also be returned, if you need to handle these
// differently it is recommended to call List first.
func (r Regexp) Wildcard() (string, bool) {
	potential := r.wildcardRecurse([]*syntax.Regexp{r.pt}, 0, 0)
	return string(potential), len(potential) > 0
}

func (r Regexp) wildcardRecurse(p []*syntax.Regexp, parentOp syntax.Op, level int) []rune {
	potential := []rune{}
	for i, s := range p {
		// Ignore (?i), etc.
		if (s.Flags & (syntax.FoldCase | syntax.DotNL)) != 0 {
			return nil
		}
		switch s.Op {

		case syntax.OpConcat:
			// Only expect concat at the root
			if len(potential) != 0 {
				return nil
			}
			potential = r.wildcardRecurse(s.Sub, s.Op, level+1)
		case syntax.OpStar:
			p := r.wildcardRecurse(s.Sub, s.Op, level+1)
			if p == nil {
				return nil
			}
			potential = append(potential, p...)
		case syntax.OpAnyCharNotNL:
			if parentOp != syntax.OpStar {
				return nil
			}
			potential = append(potential, '*')
		case syntax.OpLiteral:
			if parentOp != 0 && parentOp != syntax.OpConcat {
				return nil
			}
			potential = append(potential, s.Rune...)
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
