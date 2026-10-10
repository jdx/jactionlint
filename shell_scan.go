package jactionlint

import (
	"regexp"

	"github.com/jdx/jactionlint/v2/internal/runscript"
)

// shQuote is how a position of a POSIX shell script is quoted.
type shQuote int8

const (
	shUnquoted shQuote = iota
	shDouble
	shSingle
)

// shPlace describes where a ${{ }} placeholder sits in a shell script.
type shPlace struct {
	// Quote is the quoting of the shell at the placeholder.
	Quote shQuote
	// Cannot is true when the placeholder is somewhere a shell scanner this small cannot reason about
	// (command substitution, here-documents, comments, ...). No replacement is attempted for it.
	Cannot bool
	// Unsafe is the reason why replacing the placeholder with a variable expansion could change what
	// the script does for benign values, e.g. the value is not quoted so quoting it changes word
	// splitting. It is empty when the replacement is equivalent.
	Unsafe string
	// Split is the reason why no replacement is made for an unquoted placeholder: it is in a position of the
	// script which is not provably a single word, so quoting it would change word splitting and globbing.
	// Cannot is true then. It is empty for the placeholders which are quoted or are the value of an assignment.
	Split string
}

// splitReason explains why an unquoted placeholder is not quoted by a fix.
const splitReason = "the expression is an unquoted argument of the script, so quoting it would change word splitting and globbing when the value is meant to expand to several words. quote it by hand, or pass it through an environment variable and expand it the way the script needs"

// assignmentWordRe matches a word which starts an assignment, NAME=.
var assignmentWordRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// assignmentValue reports whether the placeholder after the word is in the value of an assignment (`NAME=${{ x }}`,
// also after other assignments of the same command), where the shell does not split the value, so the position is a
// single word.
func assignmentValue(cmd []string, word []byte) bool {
	if !assignmentWordRe.Match(word) {
		return false
	}
	for _, w := range cmd {
		if !assignmentWordRe.MatchString(w) {
			return false
		}
	}
	return true
}

// shSpan is the byte range [Start, End) of a placeholder in a script.
type shSpan struct{ Start, End int }

// analyzeShellPlaceholders scans a POSIX shell script (bash or sh) and tells for each placeholder
// how it is quoted. The scanner knows single and double quotes, backslashes, comments and the
// constructs which it cannot follow. It does not parse the shell language. Anything it does not
// understand makes the placeholders "Cannot" so that no edit is made.
func analyzeShellPlaceholders(script string, spans []shSpan) []shPlace {
	places := make([]shPlace, len(spans))
	next := 0 // index of the next placeholder to meet
	var (
		quote     = shUnquoted
		inComment bool
		inDBrack  bool // between [[ and ]]
		word      []byte
		wordPlain = true // the current word has no quote or placeholder
		globalBad bool
		// cmd are the words of the current simple command, a placeholder or a quoted part makes the word "\x00"
		cmd []string
		// arrays is the depth of the array assignments, name=( ... ), the scanner is in
		arrays int
	)
	endWord := func() {
		if len(word) > 0 || !wordPlain {
			if wordPlain {
				cmd = append(cmd, string(word))
			} else {
				cmd = append(cmd, "\x00")
			}
		}
		if wordPlain && len(word) > 0 {
			switch string(word) {
			case "[[":
				inDBrack = true
			case "]]":
				inDBrack = false
			case "case", "select", "coproc":
				globalBad = true
			}
		}
		word = word[:0]
		wordPlain = true
	}
	i := 0
	for i < len(script) {
		if next < len(spans) && i == spans[next].Start {
			p := &places[next]
			p.Quote = quote
			switch {
			case inComment:
				p.Cannot = true
			case i > 0 && script[i-1] == '\\' && quote != shSingle:
				p.Cannot = true
			case quote == shUnquoted && (arrays > 0 || inWordList(cmd)):
				// The words of a list: the shell splits the value into the items. Quoting it makes
				// one item of them, which is another program, and leaving it be is what the script
				// asked for, so there is nothing to change but the script
				p.Cannot = true
				p.Split = splitReason
			case quote == shUnquoted && wordPlain && assignmentValue(cmd, word):
				p.Unsafe = "the value is not quoted so quoting it changes the text of the script"
			case quote == shUnquoted:
				p.Cannot = true
				p.Split = splitReason
			}
			if inDBrack && quote != shSingle {
				if p.Unsafe == "" {
					p.Unsafe = "the value is in [[ ]] where quoting changes the meaning of patterns"
				}
			}
			wordPlain = false
			i = spans[next].End
			next++
			continue
		}
		c := script[i]
		if inComment {
			if c == '\n' {
				inComment = false
			}
			i++
			continue
		}
		switch quote {
		case shSingle:
			if c == '\'' {
				quote = shUnquoted
			}
			i++
			continue
		case shDouble:
			switch c {
			case '"':
				quote = shUnquoted
			case '\\':
				// A backslash escapes the next character. A placeholder after it is detected through
				// script[i-1] when the loop gets there.
				if !(next < len(spans) && spans[next].Start == i+1) {
					i++
				}
			case '`':
				globalBad = true
			case '$':
				if i+1 < len(script) && script[i+1] == '(' {
					globalBad = true
				}
			}
			i++
			continue
		}

		// Unquoted
		switch c {
		case ' ', '\t', '\r':
			endWord()
		case '\n', ';', '&', '|', '(', ')', '{', '}':
			endWord()
			cmd = cmd[:0]
			switch {
			case c == '(' && i > 0 && script[i-1] == '=':
				arrays++
			case c == ')' && arrays > 0:
				arrays--
			}
			if c == '(' && i > 0 && (script[i-1] == '<' || script[i-1] == '>' || script[i-1] == '$') {
				globalBad = true // process substitution and command substitution
			}
		case '<':
			if i+1 < len(script) && script[i+1] == '<' {
				globalBad = true // here-document and here-string
			}
			endWord()
		case '>':
			endWord()
		case '#':
			if len(word) == 0 && wordPlain {
				inComment = true
			} else {
				word = append(word, c)
			}
		case '\\':
			wordPlain = false
			if !(next < len(spans) && spans[next].Start == i+1) {
				i++ // the escaped character is literal
			}
		case '\'':
			wordPlain = false
			quote = shSingle
		case '"':
			wordPlain = false
			quote = shDouble
		case '`':
			globalBad = true
		case '$':
			if i+1 < len(script) && (script[i+1] == '(' || script[i+1] == '\'' || script[i+1] == '"') {
				globalBad = true
			}
			word = append(word, c)
		default:
			word = append(word, c)
		}
		i++
	}
	endWord()
	for k := next; k < len(spans); k++ {
		places[k].Cannot = true // the scanner skipped it, so it does not know where it is
	}
	if quote != shUnquoted {
		globalBad = true // unterminated quote
	}
	if globalBad {
		for i := range places {
			places[i].Cannot = true
			places[i].Split = "" // the script is not understood, so the position is not known either
		}
		return places
	}
	// The scanner above does not know the shell grammar. The parser does: an unquoted placeholder in the words
	// of a for/select list or of an array assignment is meant to be split, wherever it sits (after braces,
	// prefix assignments, ${var} words, ...). A script which does not parse is not touched.
	parsed, err := runscript.Analyze(script, "bash")
	for i := range places {
		if places[i].Cannot || places[i].Quote != shUnquoted {
			continue
		}
		if err != nil || parsed.InSplitList(spans[i].Start) {
			places[i].Cannot = true
			if places[i].Unsafe == "" {
				places[i].Split = splitReason
			}
		}
	}
	return places
}

// inWordList reports whether the words of the command so far are the start of the list of "for NAME in" or
// "select NAME in", where the unquoted words after "in" are split and expanded into the items.
func inWordList(cmd []string) bool {
	for len(cmd) > 0 {
		switch cmd[0] {
		case "do", "then", "else", "elif", "if", "while", "until", "!", "time":
			cmd = cmd[1:]
			continue
		}
		break
	}
	return len(cmd) >= 3 && (cmd[0] == "for" || cmd[0] == "select") && cmd[2] == "in"
}
