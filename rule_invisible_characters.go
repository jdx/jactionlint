package jactionlint

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file implements the "invisible-characters" rule. It looks at the raw bytes of a file instead
// of the syntax tree: an invisible character is a problem wherever it is, also in a file which
// does not parse, in a comment or in a key. That is why it is a source rule (see sourceRules)
// and not a Rule which visits a Workflow. It checks workflows and Dependabot configurations.

// sourceRule checks the raw content of a file, whatever kind of file it is. It returns the errors
// it found. The linter calls every registered source rule for each file; a rule that is switched
// off by the configuration reports nothing (its errors are dropped when they are annotated).
type sourceRule struct {
	// name is the name of the rule. Errors of the rule have it as Error.Kind.
	name  string
	check func(src []byte, cfg *Config) []*Error
}

// sourceRules is the list of the rules which look at the raw content of every checked file.
var sourceRules []sourceRule

// registerSourceRule registers a rule which checks the raw content of files. It is meant to be
// called from init functions.
func registerSourceRule(name string, check func(src []byte, cfg *Config) []*Error) {
	for _, r := range sourceRules {
		if r.name == name {
			panic("jactionlint: source rule " + name + " is registered twice")
		}
	}
	sourceRules = append(sourceRules, sourceRule{name, check})
}

// checkSourceRules runs the source rules on a file.
func checkSourceRules(src []byte, cfg *Config) []*Error {
	var ret []*Error
	for _, r := range sourceRules {
		ret = append(ret, r.check(src, cfg)...)
	}
	return ret
}

// invisibleNames are the names of the invisible characters which are known by name. The other
// characters are described by their kind.
var invisibleNames = map[rune]string{
	0x00AD: "SOFT HYPHEN", 0x034F: "COMBINING GRAPHEME JOINER", 0x061C: "ARABIC LETTER MARK",
	0x115F: "HANGUL CHOSEONG FILLER", 0x1160: "HANGUL JUNGSEONG FILLER",
	0x17B4: "KHMER VOWEL INHERENT AQ", 0x17B5: "KHMER VOWEL INHERENT AA",
	0x180B: "MONGOLIAN FREE VARIATION SELECTOR ONE", 0x180C: "MONGOLIAN FREE VARIATION SELECTOR TWO",
	0x180D: "MONGOLIAN FREE VARIATION SELECTOR THREE", 0x180E: "MONGOLIAN VOWEL SEPARATOR",
	0x180F: "MONGOLIAN FREE VARIATION SELECTOR FOUR",
	0x200B: "ZERO WIDTH SPACE", 0x200C: "ZERO WIDTH NON-JOINER", 0x200D: "ZERO WIDTH JOINER",
	0x200E: "LEFT-TO-RIGHT MARK", 0x200F: "RIGHT-TO-LEFT MARK",
	0x2028: "LINE SEPARATOR", 0x2029: "PARAGRAPH SEPARATOR",
	0x202A: "LEFT-TO-RIGHT EMBEDDING", 0x202B: "RIGHT-TO-LEFT EMBEDDING", 0x202C: "POP DIRECTIONAL FORMATTING",
	0x202D: "LEFT-TO-RIGHT OVERRIDE", 0x202E: "RIGHT-TO-LEFT OVERRIDE",
	0x2060: "WORD JOINER", 0x2061: "FUNCTION APPLICATION", 0x2062: "INVISIBLE TIMES",
	0x2063: "INVISIBLE SEPARATOR", 0x2064: "INVISIBLE PLUS",
	0x2066: "LEFT-TO-RIGHT ISOLATE", 0x2067: "RIGHT-TO-LEFT ISOLATE", 0x2068: "FIRST STRONG ISOLATE",
	0x2069: "POP DIRECTIONAL ISOLATE",
	0x206A: "INHIBIT SYMMETRIC SWAPPING", 0x206B: "ACTIVATE SYMMETRIC SWAPPING",
	0x206C: "INHIBIT ARABIC FORM SHAPING", 0x206D: "ACTIVATE ARABIC FORM SHAPING",
	0x206E: "NATIONAL DIGIT SHAPES", 0x206F: "NOMINAL DIGIT SHAPES",
	0x3164: "HANGUL FILLER", 0xFEFF: "ZERO WIDTH NO-BREAK SPACE (BYTE ORDER MARK)", 0xFFA0: "HALFWIDTH HANGUL FILLER",
	0xFFF9: "INTERLINEAR ANNOTATION ANCHOR", 0xFFFA: "INTERLINEAR ANNOTATION SEPARATOR",
	0xFFFB:  "INTERLINEAR ANNOTATION TERMINATOR",
	0xE0001: "LANGUAGE TAG",
	0x0085:  "NEXT LINE",
}

// isBidiControl reports whether the character changes the direction in which the text around it is
// displayed. The overrides, embeddings and isolates are the ones used to disguise source code.
func isBidiControl(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// invisibleName describes a character for a message: "U+202E RIGHT-TO-LEFT OVERRIDE".
func invisibleName(r rune) string {
	name, ok := invisibleNames[r]
	switch {
	case ok:
	case r < 0x20 || r == 0x7F:
		name = "CONTROL CHARACTER"
	case r >= 0x80 && r < 0xA0:
		name = "CONTROL CHARACTER"
	case r >= 0xFE00 && r <= 0xFE0F:
		name = fmt.Sprintf("VARIATION SELECTOR-%d", r-0xFE00+1)
	case r >= 0xE0100 && r <= 0xE01EF:
		name = fmt.Sprintf("VARIATION SELECTOR-%d", r-0xE0100+17)
	case r >= 0xE0020 && r <= 0xE007E:
		name = "TAG CHARACTER " + string(r-0xE0000)
	case r == 0xE007F:
		name = "CANCEL TAG"
	case unicode.Is(unicode.Cf, r):
		name = "FORMAT CHARACTER"
	default:
		name = "INVISIBLE CHARACTER"
	}
	return fmt.Sprintf("U+%04X %s", r, name)
}

// visibleFormatChars are format characters (category Cf) which are displayed: they are written
// before a number or a text in the scripts which need them.
func visibleFormatChar(r rune) bool {
	switch {
	case r >= 0x0600 && r <= 0x0605, r == 0x06DD, r == 0x070F, r == 0x0890, r == 0x0891, r == 0x08E2, r == 0x110BD, r == 0x110CD:
		return true
	}
	return false
}

// isInvisible reports whether the character is displayed as nothing (or changes how other text is
// displayed) and so is not what a reader of the file expects to be there. It is never a letter,
// a mark or a symbol of a script: only controls, format characters, variation selectors, fillers
// and line separators are invisible, so text in any language is not affected.
func isInvisible(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return false
	case r < 0x20 || r == 0x7F, r >= 0x80 && r < 0xA0:
		return true
	case r == 0x2028 || r == 0x2029:
		return true
	case r == 0x115F || r == 0x1160 || r == 0x3164 || r == 0xFFA0:
		return true
	case unicode.Is(unicode.Cf, r):
		return !visibleFormatChar(r)
	case unicode.Is(unicode.Variation_Selector, r), unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r):
		return true
	}
	return false
}

func isRTLLetter(r rune) bool {
	return unicode.In(r, unicode.Arabic, unicode.Hebrew, unicode.Syriac, unicode.Thaana, unicode.Nko, unicode.Samaritan,
		unicode.Mandaic, unicode.Adlam)
}

// joinerScripts are the scripts in which the zero width (non-)joiner is part of correct spelling.
var joinerScripts = []*unicode.RangeTable{
	unicode.Arabic, unicode.Syriac, unicode.Thaana, unicode.Nko, unicode.Mongolian, unicode.Devanagari, unicode.Bengali,
	unicode.Gurmukhi, unicode.Gujarati, unicode.Oriya, unicode.Tamil, unicode.Telugu, unicode.Kannada, unicode.Malayalam,
	unicode.Sinhala, unicode.Khmer, unicode.Myanmar, unicode.Tibetan, unicode.Lao, unicode.Thai,
}

func inJoinerScript(r rune) bool {
	return (unicode.IsLetter(r) || unicode.IsMark(r)) && unicode.In(r, joinerScripts...)
}

// isEmojiModifier reports whether the character is a skin tone modifier or a variation selector
// which can follow an emoji.
func isEmojiModifier(r rune) bool {
	return (r >= 0x1F3FB && r <= 0x1F3FF) || r == 0xFE0F || r == 0xFE0E
}

// isEmojiBase reports whether the character can be an element of an emoji sequence.
func isEmojiBase(r rune) bool {
	return unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || r == 0x203C || r == 0x2049 || r == 0x3030 || r == 0x303D || r == 0x2139
}

func runeBefore(src []byte, i int) (rune, int) {
	if i <= 0 {
		return utf8.RuneError, 0
	}
	return utf8.DecodeLastRune(src[:i])
}

func runeAfter(src []byte, i int) (rune, int) {
	if i >= len(src) {
		return utf8.RuneError, 0
	}
	return utf8.DecodeRune(src[i:])
}

// emojiBefore returns the character before position i which is the base of an emoji sequence,
// skipping modifiers.
func emojiBefore(src []byte, i int) (rune, bool) {
	for {
		r, n := runeBefore(src, i)
		if n == 0 {
			return 0, false
		}
		if isEmojiModifier(r) {
			i -= n
			continue
		}
		return r, isEmojiBase(r) || r == 0x20E3
	}
}

// legitimate reports whether the invisible character r at src[i:i+n] is a part of correct text
// rather than a hidden character: the byte order mark at the start of a file, the joiners and variation
// selectors of emoji, the joiners of the scripts which need them, the variation selectors of CJK and
// Mongolian text, the tags of flags, and the direction marks next to right-to-left letters.
func legitimate(src []byte, i, n int, r rune) bool {
	prev, pn := runeBefore(src, i)
	next, nn := runeAfter(src, i+n)
	switch {
	case r == 0xFEFF:
		return i == 0
	case r == 0x200D || r == 0x200C:
		if r == 0x200D {
			if b, ok := emojiBefore(src, i); ok && b != 0x20E3 && nn > 0 && isEmojiBase(next) {
				return true
			}
		}
		// The joiners must have a letter on both sides, which are of a script that needs them
		for j := i; j > 0; { // skip other joiners when looking for the letter before
			p, k := runeBefore(src, j)
			if p != 0x200C && p != 0x200D {
				prev, pn = p, k
				break
			}
			j -= k
		}
		return pn > 0 && nn > 0 && inJoinerScript(prev) && inJoinerScript(next)
	case r == 0xFE0F || r == 0xFE0E:
		switch {
		case pn == 0:
			return false
		case isEmojiBase(prev) || (prev >= 0x2190 && prev <= 0x21FF) || unicode.Is(unicode.Han, prev):
			return true
		case (prev >= '0' && prev <= '9') || prev == '#' || prev == '*':
			return r == 0xFE0F && next == 0x20E3 // keycap sequence
		}
		return false
	case (r >= 0xFE00 && r <= 0xFE0D) || (r >= 0xE0100 && r <= 0xE01EF):
		// Variation sequences of CJK ideographs, and of mathematical symbols
		return pn > 0 && (unicode.Is(unicode.Han, prev) || (r == 0xFE00 && unicode.Is(unicode.Sm, prev)))
	case r >= 0x180B && r <= 0x180F:
		return pn > 0 && unicode.Is(unicode.Mongolian, prev) && (r != 0x180E || (nn > 0 && unicode.Is(unicode.Mongolian, next)))
	case r >= 0xE0020 && r <= 0xE007F:
		// The tags of a subdivision flag follow the waving black flag and end with a cancel tag
		start := i
		for {
			p, k := runeBefore(src, start)
			if k == 0 || p < 0xE0020 || p > 0xE007E {
				if p != 0x1F3F4 {
					return false
				}
				break
			}
			start -= k
		}
		end := i
		last := rune(0)
		for {
			q, k := runeAfter(src, end)
			if k == 0 || q < 0xE0020 || q > 0xE007F {
				break
			}
			last = q
			end += k
		}
		return last == 0xE007F || r == 0xE007F
	case r == 0x061C || r == 0x200E || r == 0x200F:
		// Direction marks belong to right-to-left text: they have a letter of it next to them
		for _, dir := range []int{-1, 1} {
			j := i
			if dir > 0 {
				j = i + n
			}
			for {
				var q rune
				var k int
				if dir < 0 {
					q, k = runeBefore(src, j)
				} else {
					q, k = runeAfter(src, j)
				}
				if k == 0 {
					break
				}
				if q == 0x061C || q == 0x200E || q == 0x200F {
					j += dir * k
					continue
				}
				if isRTLLetter(q) {
					return true
				}
				break
			}
		}
		return false
	}
	return false
}

// invisibleRun is a sequence of adjacent invisible characters.
type invisibleRun struct {
	start, end int // byte offsets
	chars      []rune
}

// findInvisibleRuns returns the invisible characters of the source which are not legitimate, with
// adjacent characters grouped.
func findInvisibleRuns(src []byte) []invisibleRun {
	var runs []invisibleRun
	// Fast path: ASCII without controls, which is nearly every file
	ascii := true
	for _, b := range src {
		if b >= 0x80 || (b < 0x20 && b != '\t' && b != '\n' && b != '\r') || b == 0x7F {
			ascii = false
			break
		}
	}
	if ascii {
		return nil
	}
	for i := 0; i < len(src); {
		r, n := utf8.DecodeRune(src[i:])
		if r == utf8.RuneError && n <= 1 {
			i++
			continue
		}
		if !isInvisible(r) || legitimate(src, i, n, r) {
			i += n
			continue
		}
		if k := len(runs) - 1; k >= 0 && runs[k].end == i {
			runs[k].end = i + n
			runs[k].chars = append(runs[k].chars, r)
		} else {
			runs = append(runs, invisibleRun{start: i, end: i + n, chars: []rune{r}})
		}
		i += n
	}
	return runs
}

// invisibleLineContext tells what a line of a YAML file belongs to.
type invisibleLineContext struct {
	// block is the key of the block scalar the line is the content of, or "".
	block   string
	inBlock bool
	// key is the key of the mapping entry the line starts, or "".
	key string
	// keyEnd is the byte offset of the ":" of the key in the line, or -1.
	keyEnd int
}

// classifyLines computes the context of every line, with a lightweight reading of the YAML
// structure: which lines are the content of a block scalar and the key which owns it.
func classifyLines(lines []string) []invisibleLineContext {
	ctx := make([]invisibleLineContext, len(lines))
	inBlock := false
	blockKey := ""
	blockIndent := 0
	for i, line := range lines {
		ctx[i].keyEnd = -1
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if inBlock {
			if strings.TrimSpace(line) == "" || indent > blockIndent {
				ctx[i].inBlock, ctx[i].block = true, blockKey
				continue
			}
			inBlock = false
		}
		// "- key: value" and "key: value"
		rest := trimmed
		off := indent
		for strings.HasPrefix(rest, "- ") {
			rest = strings.TrimLeft(rest[2:], " ")
			off = len(line) - len(rest)
		}
		key, ok := yamlKey(rest)
		if !ok {
			continue
		}
		ctx[i].key = key
		ctx[i].keyEnd = off + strings.Index(rest, ":")
		if q := strings.IndexAny(rest, "\"'"); q == 0 {
			// Quoted key: the colon follows the closing quote
			ctx[i].keyEnd = off + len(key) + 2 + strings.Index(rest[len(key)+2:], ":")
		}
		val := rest[strings.Index(rest, ":")+1:]
		if q := strings.IndexAny(rest, "\"'"); q == 0 {
			val = rest[len(key)+2+strings.Index(rest[len(key)+2:], ":")+1:]
		}
		if j := strings.Index(val, " #"); j >= 0 {
			val = val[:j]
		}
		val = strings.TrimSpace(val)
		if len(val) > 0 && (val[0] == '|' || val[0] == '>') && strings.Trim(val[1:], "+-0123456789") == "" {
			inBlock, blockKey, blockIndent = true, key, off
		}
	}
	return ctx
}

// yamlKey returns the key if the text starts a "key:" mapping entry.
func yamlKey(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	if s[0] == '"' || s[0] == '\'' {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", false
		}
		rest := s[end+2:]
		if strings.HasPrefix(rest, ":") {
			return s[1 : end+1], true
		}
		return "", false
	}
	i := strings.Index(s, ":")
	for i >= 0 {
		if i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '\t' {
			k := strings.TrimSpace(s[:i])
			if k == "" || strings.ContainsAny(k, " \t{}[]") && !strings.HasPrefix(k, "${{") {
				return "", false
			}
			return k, true
		}
		j := strings.Index(s[i+1:], ":")
		if j < 0 {
			break
		}
		i += 1 + j
	}
	return "", false
}

// describePlace returns the place of a column of a line, as a noun phrase for a message.
func describePlace(c invisibleLineContext, line string, byteOff int, inComment, inExpr bool) string {
	switch {
	case inComment:
		return "a comment"
	case inExpr:
		return "an expression"
	}
	key := c.key
	if c.inBlock {
		key = c.block
	} else if c.keyEnd >= 0 && byteOff <= c.keyEnd {
		return "the key " + fmt.Sprintf("%q", key)
	}
	switch key {
	case "run":
		return "a run: script"
	case "script":
		return "a script"
	case "uses":
		return "a uses: reference"
	case "if":
		return "a condition"
	case "shell":
		return "a shell: value"
	}
	if key != "" {
		return fmt.Sprintf("the value of %q", key)
	}
	return "a value"
}

func checkInvisibleCharacters(src []byte, cfg *Config) []*Error {
	if !cfg.RuleEnabled("invisible-characters") {
		return nil
	}
	runs := findInvisibleRuns(src)
	if len(runs) == 0 {
		return nil
	}
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	ctx := classifyLines(lines)
	comments := NewCommentIndex(src)

	// Byte offsets of the lines
	lineStart := make([]int, len(lines))
	off := 0
	for i, l := range strings.Split(string(src), "\n") {
		lineStart[i] = off
		off += len(l) + 1
	}
	var errs []*Error
	for _, run := range runs {
		line := sortSearchLine(lineStart, run.start)
		col := utf8.RuneCount(src[lineStart[line]:run.start]) + 1
		text := lines[line]
		byteOff := run.start - lineStart[line]
		inComment := false
		if c := comments.At(line + 1); c != nil && c.Column < col {
			inComment = true
		}
		inExpr := insideExpression(src, run.start)

		place := describePlace(ctx[line], text, byteOff, inComment, inExpr)
		var names []string
		bidi := false
		for i, r := range run.chars {
			if i < 3 {
				names = append(names, invisibleName(r))
			}
			bidi = bidi || isBidiControl(r) && r != 0x061C && r != 0x200E && r != 0x200F
		}
		what := "invisible character " + strings.Join(names, ", ")
		if len(run.chars) > 3 {
			what = fmt.Sprintf("%d invisible characters (%s and %d more)", len(run.chars), strings.Join(names, ", "), len(run.chars)-3)
		} else if len(run.chars) > 1 {
			what = fmt.Sprintf("%d invisible characters (%s)", len(run.chars), strings.Join(names, ", "))
		}
		why := "it is not shown by editors or by the diff view of GitHub, so it can hide what the text really is"
		if bidi {
			why = "it changes the order in which the text around it is displayed, so the code can run differently from how it reads"
		}
		msg := fmt.Sprintf("%s in %s: %s. remove it", what, place, why)
		if strings.HasPrefix(place, "the value of") || place == "a value" {
			msg += `, or write it as an escape in a double quoted string such as "\u200b" if it is needed`
		}
		e := &Error{
			Message:   msg,
			Line:      line + 1,
			Column:    col,
			EndLine:   line + 1,
			EndColumn: col + len(run.chars),
			Kind:      "invisible-characters",
			ID:        "invisible-characters",
			Fix: &Fix{
				Description: "Remove the invisible characters",
				Edits:       []TextEdit{{Start: run.start, End: run.end}},
			},
		}
		errs = append(errs, e)
	}
	return errs
}

// sortSearchLine returns the index of the line which holds the byte offset.
func sortSearchLine(lineStart []int, off int) int {
	lo, hi := 0, len(lineStart)
	for lo < hi {
		mid := (lo + hi) / 2
		if lineStart[mid] <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}

// insideExpression reports whether the offset is inside "${{ ... }}": an opening before it which is
// not closed. An expression which spans lines is understood.
func insideExpression(src []byte, off int) bool {
	head := src[:off]
	i := bytes.LastIndex(head, []byte("${{"))
	if i < 0 {
		return false
	}
	return !bytes.Contains(head[i:], []byte("}}"))
}

func init() {
	registerRules(
		RuleInfo{
			ID: "invisible-characters", Group: RuleGroupSecurity, Summary: "A file contains an invisible or bidirectional control character which hides what the text says.",
			DefaultLevel: SeverityError, Profile: ProfileDefault, Fixable: true, DocsAnchor: "check-invisible-characters",
		},
	)
	registerSourceRule("invisible-characters", checkInvisibleCharacters)
}
