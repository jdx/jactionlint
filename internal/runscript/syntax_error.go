package runscript

import (
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// SyntaxProblem is a syntax error of a script, as far as it is one for bash itself.
type SyntaxProblem struct {
	// Offset is the byte offset in the script where the parser stopped (the opening token for an unclosed one).
	Offset int
	// Text is the message of the parser, such as "`if` statement must end with `fi`".
	Text string
}

// SyntaxError parses the script as bash and returns the syntax error that the shell reports as well, or nil. The
// script must not contain `${{ }}` expressions any more (see the caller), and the offset of the problem is a byte
// offset in it. The point is not to find every syntax error. It is to report none that bash accepts, so it returns
// nil whenever the parser and bash may disagree:
//
//   - the parser fails with something other than a syntax error (a construct that exists only in another shell, a
//     failure inside the parser),
//   - the message is not one of the structural errors that bash finds while it reads the script: an unclosed quote,
//     a missing "fi", "done" or "esac", a keyword where it does not belong, a command without its operand. Bash
//     checks the other kinds only when it runs the command (a bad substitution such as "${x<}", the syntax of an
//     arithmetic expression, the name of an array), and a here-document that is not closed is a warning for it,
//   - the problem is after the first here-document, backquote or "((" of the script (see [lazilyParsedFrom]). Bash
//     does not read the body of a here-document, it parses a command in backquotes only when it runs it, and it
//     evaluates an arithmetic expression only then, so a problem the parser finds in there is not a syntax error
//     of the script. (A here-document that is closed is skipped, too: the parser reads its expansions, the shell
//     does not.)
func SyntaxError(script string) (problem *SyntaxProblem) {
	defer func() {
		if r := recover(); r != nil {
			problem = nil
		}
	}()
	parser := parserPool.Get().(*syntax.Parser)
	_, err := parser.Parse(strings.NewReader(script), "")
	_, _ = parser.Parse(strings.NewReader(""), "")
	parserPool.Put(parser)
	var pe syntax.ParseError
	if err == nil || !errors.As(err, &pe) || !pe.Pos.IsValid() || !structuralSyntaxError(pe.Text) {
		return nil
	}
	off := int(pe.Pos.Offset())
	if off < 0 || off > len(script) {
		return nil
	}
	if limit := lazilyParsedFrom(script); limit >= 0 && off >= limit {
		return nil
	}
	if badExpansion(script) {
		return nil
	}
	return &SyntaxProblem{Offset: off, Text: pe.Text}
}

// structuralSyntaxErrors are the messages of the parser for the mistakes which bash reports when it reads the script,
// as parts of the message. The other messages are not reported: see [SyntaxError].
var structuralSyntaxErrors = []string{
	"reached EOF without closing quote",
	"without matching `", // reached EOF without matching `$(` with `)`, reached `)` without matching `[[` with `]]`
	"statement must end with",
	"can only be used to end",
	"can only be used to close",
	"can only be used in an `if`", // `then`, `elif` and `else`
	"can only be used in a loop",
	"can only be used in a case clause",
	"must be followed by a statement",
	"must be followed by `then`",
	"must be followed by `do`",
	"can only immediately follow a statement",
	"can only contain words and redirects",
}

func structuralSyntaxError(text string) bool {
	// "reached X without matching Y" is reported only when X is the end of the script. A closer found earlier ("${a:(-4}",
	// "${a[0}") is a bad substitution for bash, which fails when it runs the command and finds where the expansion
	// ends differently from the parser.
	if strings.Contains(text, "without matching `") && !strings.HasPrefix(text, "reached EOF without matching `") {
		return false
	}
	// A redirection without its file ("echo a >"). The operators of a [[ ]] test ("[[ a =~ ]]") are not read the way bash
	// does: it accepts "=~&&" where the parser wants a word
	if strings.Contains(text, "must be followed by a word") {
		return strings.HasPrefix(text, "`>") || strings.HasPrefix(text, "`<") || strings.HasPrefix(text, "`&>")
	}
	for _, s := range structuralSyntaxErrors {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// lazilyParsedFrom returns the offset of the first here-document operator ("<<", not "<<<"), backquote or "((" (an
// arithmetic expansion or command) of the script, or -1 when it has none. Bash reads none of them as the parser does: the
// body of a here-document is text for it, a command in backquotes is parsed when it runs, and an arithmetic expression
// is evaluated when it runs. An operator in a string or a comment counts as well, which only makes the check stricter.
func lazilyParsedFrom(script string) int {
	limit := strings.IndexByte(script, '`')
	if i := strings.Index(script, "(("); i >= 0 && (limit < 0 || i < limit) {
		limit = i
	}
	for i := 0; i+1 < len(script); i++ {
		if script[i] != '<' || script[i+1] != '<' {
			continue
		}
		if i+2 < len(script) && script[i+2] == '<' {
			i += 2 // <<<
			continue
		}
		if limit < 0 || i < limit {
			limit = i
		}
		break
	}
	return limit
}

// badExpansion reports whether the script has "${" that is not followed by the start of a parameter name or one of
// the operators that come right after the brace ("${|x}" and "${;x}" are bad substitutions for bash, which finds where
// the expansion ends differently from the parser). The check is for the whole script because the parser's error is
// not where the expansion is.
func badExpansion(script string) bool {
	for i := 0; ; {
		j := strings.Index(script[i:], "${")
		if j < 0 {
			return false
		}
		i += j + 2
		if i >= len(script) {
			return true
		}
		switch c := script[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '#', c == '!', c == '@', c == '*', c == '?', c == '$', c == '-':
		default:
			return true
		}
	}
}
