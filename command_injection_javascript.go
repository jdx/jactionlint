package jactionlint

import (
	"regexp"
	"sort"
	"strings"
)

// This file finds untrusted input that the script of actions/github-script hands to a shell. There is no
// JavaScript parser in jactionlint, so the scan is small and conservative: it reads the structure that matters
// (strings, comments, template literals and the parentheses of calls) and reports only the calls it can follow.
//
//   - The sources are `process.env.NAME` (when the step, its job or the workflow set NAME to an untrusted
//     expression, the same judgement as for a shell variable), `context.payload.*` (the event, which the
//     table of untrusted inputs knows as github.event.*), and the constants of the script that were set from
//     them (`const body = process.env.BODY`, `const { title } = context.payload.issue`, `const cmd = ...${body}`).
//   - The sinks are the calls of child_process that run a shell (`exec`, `execSync`, and `spawn`, `execFile` and
//     their sync forms with `shell: true`), a shell started with `-c` (`spawn('bash', ['-c', cmd])`), and
//     `exec.exec(cmd)` of @actions/exec when the untrusted value is the program that runs (it is the start of
//     the command line). A value that is an argument in the array (`exec.exec('git', ['checkout', branch])`) is
//     not a finding.
//   - A value is not reported when it is wrapped by a function (`Number(x)`, `shellEscape(x)`), is followed by a
//     method that may clean it (`x.replace(...)`), or the script tests it with an anchored regular expression
//     (`/^[a-z0-9-]+$/.test(x)`).
//
// An expression of the workflow written into the script (`${{ github.event.issue.title }}`) is blanked first: it is
// a finding of template-injection.

// Classes of the bytes of a script.
const (
	jsCode     byte = iota
	jsText          // a string, the text of a template literal, a comment or a regular expression
	jsTplOpen       // the `${` of a template literal
	jsTplClose      // the `}` that ends a `${`
)

// jsClassify returns the class of every byte of the script.
func jsClassify(src string) []byte {
	n := len(src)
	cls := make([]byte, n)
	type mode struct {
		tmpl   bool
		braces int
	}
	stack := []mode{{}}
	prevSig := func(i int) (byte, int) { // the last byte before i that is not a space, and its offset
		for j := i - 1; j >= 0; j-- {
			if src[j] != ' ' && src[j] != '\t' && src[j] != '\n' && src[j] != '\r' {
				return src[j], j
			}
		}
		return 0, -1
	}
	for i := 0; i < n; {
		m := &stack[len(stack)-1]
		c := src[i]
		if m.tmpl {
			cls[i] = jsText
			switch {
			case c == '\\':
				if i+1 < n {
					cls[i+1] = jsText
				}
				i += 2
			case c == '`':
				stack = stack[:len(stack)-1]
				i++
			case c == '$' && i+1 < n && src[i+1] == '{':
				cls[i], cls[i+1] = jsTplOpen, jsTplOpen
				stack = append(stack, mode{})
				i += 2
			default:
				i++
			}
			continue
		}
		switch c {
		case '\'', '"':
			j := i + 1
			for j < n && src[j] != c && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			for k := i; k <= j && k < n; k++ {
				cls[k] = jsText
			}
			i = j + 1
		case '`':
			cls[i] = jsText
			stack = append(stack, mode{tmpl: true})
			i++
		case '{':
			m.braces++
			i++
		case '}':
			if m.braces == 0 && len(stack) > 1 {
				cls[i] = jsTplClose
				stack = stack[:len(stack)-1]
			} else if m.braces > 0 {
				m.braces--
			}
			i++
		case '/':
			switch {
			case i+1 < n && src[i+1] == '/':
				j := strings.IndexByte(src[i:], '\n')
				if j < 0 {
					j = n - i
				}
				for k := i; k < i+j; k++ {
					cls[k] = jsText
				}
				i += j
			case i+1 < n && src[i+1] == '*':
				j := strings.Index(src[i+2:], "*/")
				end := n
				if j >= 0 {
					end = i + 2 + j + 2
				}
				for k := i; k < end; k++ {
					cls[k] = jsText
				}
				i = end
			default:
				p, at := prevSig(i)
				if jsRegexAllowed(src, p, at) {
					if end := jsRegexEnd(src, i); end > i {
						for k := i; k < end; k++ {
							cls[k] = jsText
						}
						i = end
						continue
					}
				}
				i++
			}
		default:
			i++
		}
	}
	return cls
}

// jsRegexAllowed reports whether a `/` after the byte p (at offset at) starts a regular expression literal.
func jsRegexAllowed(src string, p byte, at int) bool {
	if at < 0 {
		return true
	}
	switch p {
	case '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', '}', ';', '+', '-', '*', '%', '<', '>', '~', '^':
		return true
	}
	if p == '_' || p == '$' || (p >= 'a' && p <= 'z') || (p >= 'A' && p <= 'Z') || (p >= '0' && p <= '9') {
		j := at
		for j >= 0 && (src[j] == '_' || src[j] == '$' || (src[j] >= 'a' && src[j] <= 'z') || (src[j] >= 'A' && src[j] <= 'Z') || (src[j] >= '0' && src[j] <= '9')) {
			j--
		}
		switch src[j+1 : at+1] {
		case "return", "typeof", "case", "in", "of", "delete", "void", "throw", "new", "else", "do", "yield", "await":
			return true
		}
	}
	return false
}

// jsRegexEnd returns the offset after the regular expression literal that starts at i, or i when it is not one.
func jsRegexEnd(src string, i int) int {
	inClass := false
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '\n':
			return i
		case '\\':
			j++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				return j + 1
			}
		}
	}
	return i
}

// jsSource is a place the untrusted input comes from.
type jsSource struct {
	// key identifies the source: the variable ("env:BODY") or the property of the payload ("payload:issue.title").
	key string
	d   data
}

// jsRef is a read of a source in the script.
type jsRef struct {
	start, end int
	text       string
	src        *jsSource
}

var (
	reJSEnvDot     = regexp.MustCompile(`\bprocess\s*\.\s*env\s*\.\s*([A-Za-z_][A-Za-z0-9_]*)`)
	reJSEnvBracket = regexp.MustCompile(`\bprocess\s*\.\s*env\s*\[\s*['"]([A-Za-z_][A-Za-z0-9_]*)['"]\s*\]`)
	reJSPayload    = regexp.MustCompile(`\b(?:github\.)?context\.payload((?:\s*\??\.\s*[A-Za-z_$][A-Za-z0-9_$]*)+)`)
	reJSIdent      = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)
	reJSDeclSimple = regexp.MustCompile(`\b(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	reJSDeclDestr  = regexp.MustCompile(`\b(?:const|let|var)\s*\{([^}]*)\}\s*=`)
)

// jsScan holds what is known about one script while it is scanned.
type jsScan struct {
	src, clean string
	cls        []byte
	// taint judges a variable of the environment of the step.
	env func(name string) (data, bool)
	// aliases maps a constant of the script to the source it holds.
	aliases map[string]*jsSource
	sources map[string]*jsSource
	// validated holds the keys of the sources the script tests with an anchored regular expression.
	validated map[string]bool
}

// jsHit is a place where untrusted input reaches a shell.
type jsHit struct {
	start, end int
	// kind is "shell" (a call that runs a shell), "command" (the untrusted value is the program) or "shellc" (a shell
	// started with -c), call names the callee.
	kind, call string
	d          data
}

// scanJSCommandSinks finds the calls of the script that run untrusted input in a shell. env judges the variables
// of the environment of the step. exprs are the byte ranges of the `${{ }}` expressions of the workflow.
func scanJSCommandSinks(src string, env func(name string) (data, bool)) []jsHit {
	clean := blankExprs(src)
	sc := &jsScan{src: src, clean: clean, cls: jsClassify(clean), env: env, aliases: map[string]*jsSource{}, sources: map[string]*jsSource{}, validated: map[string]bool{}}
	sc.collectAliases()
	sc.collectValidated()
	return sc.findSinks()
}

// blankExprs replaces every `${{ }}` expression by underscores of the same length.
func blankExprs(src string) string {
	if !strings.Contains(src, "${{") {
		return src
	}
	b := []byte(src)
	for i := 0; i < len(src); {
		j := strings.Index(src[i:], "${{")
		if j < 0 {
			break
		}
		start := i + j
		end := exprClose(src, start+3)
		if end < 0 {
			break
		}
		end += 2
		for k := start; k < end; k++ {
			if b[k] != '\n' {
				b[k] = '_'
			}
		}
		i = end
	}
	return string(b)
}

func (sc *jsScan) code(i int) bool { return i >= 0 && i < len(sc.cls) && sc.cls[i] == jsCode }

// source returns the source with the key, judged once.
func (sc *jsScan) source(key string, judge func() (data, bool)) *jsSource {
	if s, ok := sc.sources[key]; ok {
		return s
	}
	var s *jsSource
	if d, ok := judge(); ok {
		s = &jsSource{key: key, d: d}
	}
	sc.sources[key] = s
	return s
}

func (sc *jsScan) envSource(name string) *jsSource {
	return sc.source("env:"+name, func() (data, bool) {
		d, ok := sc.env(name)
		return d, ok && d.kind == dataUntrusted
	})
}

func (sc *jsScan) payloadSource(path string) *jsSource {
	return sc.source("payload:"+path, func() (data, bool) {
		e := "github.event." + path
		return data{kind: dataUntrusted, input: e}, exprReadsUntrustedInput(e)
	})
}

// directRefs lists the reads of a source (not of a constant) in the code between two offsets.
func (sc *jsScan) directRefs(from, to int) []jsRef {
	var refs []jsRef
	add := func(m []int, src *jsSource) {
		if src != nil && sc.code(m[0]) {
			refs = append(refs, jsRef{start: m[0], end: m[1], text: sc.src[m[0]:m[1]], src: src})
		}
	}
	seg := sc.clean[from:to]
	for _, m := range reJSEnvDot.FindAllStringSubmatchIndex(seg, -1) {
		add([]int{from + m[0], from + m[1]}, sc.envSource(seg[m[2]:m[3]]))
	}
	for _, m := range reJSEnvBracket.FindAllStringSubmatchIndex(seg, -1) {
		add([]int{from + m[0], from + m[1]}, sc.envSource(seg[m[2]:m[3]]))
	}
	for _, m := range reJSPayload.FindAllStringSubmatchIndex(seg, -1) {
		path := strings.NewReplacer("?.", ".", " ", "", "\t", "", "\n", "").Replace(seg[m[2]:m[3]])
		path = strings.TrimPrefix(path, ".")
		add([]int{from + m[0], from + m[1]}, sc.payloadSource(path))
	}
	return refs
}

// identRefs lists the reads of the constants that hold a source in the code between two offsets.
func (sc *jsScan) identRefs(from, to int) []jsRef {
	if len(sc.aliases) == 0 {
		return nil
	}
	var refs []jsRef
	seg := sc.clean[from:to]
	for _, m := range reJSIdent.FindAllStringIndex(seg, -1) {
		name := seg[m[0]:m[1]]
		src, ok := sc.aliases[name]
		start := from + m[0]
		if !ok || !sc.code(start) {
			continue
		}
		if p := sc.prevByte(start); p == '.' {
			continue // a property of something else
		}
		refs = append(refs, jsRef{start: start, end: from + m[1], text: name, src: src})
	}
	return refs
}

func (sc *jsScan) prevByte(i int) byte {
	for j := i - 1; j >= 0; j-- {
		if c := sc.clean[j]; c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c
		}
	}
	return 0
}

func (sc *jsScan) refs(from, to int) []jsRef {
	refs := append(sc.directRefs(from, to), sc.identRefs(from, to)...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].start < refs[j].start })
	return refs
}

// collectAliases finds the constants that hold a source. It runs twice so that a constant built from another one
// is found whatever the order.
func (sc *jsScan) collectAliases() {
	for round := 0; round < 3; round++ {
		before := len(sc.aliases)
		for _, m := range reJSDeclSimple.FindAllStringSubmatchIndex(sc.clean, -1) {
			if !sc.code(m[0]) {
				continue
			}
			name := sc.clean[m[2]:m[3]]
			if _, ok := sc.aliases[name]; ok {
				continue
			}
			end := sc.statementEnd(m[1])
			for _, r := range sc.refs(m[1], end) {
				if !sc.sanitized(r, m[1], end) {
					sc.aliases[name] = r.src
					break
				}
			}
		}
		for _, m := range reJSDeclDestr.FindAllStringSubmatchIndex(sc.clean, -1) {
			if !sc.code(m[0]) {
				continue
			}
			end := sc.statementEnd(m[1])
			rhs := strings.TrimSpace(sc.clean[m[1]:end])
			names := sc.clean[m[2]:m[3]]
			for _, part := range strings.Split(names, ",") {
				prop, local := part, part
				if i := strings.IndexByte(part, ':'); i >= 0 {
					prop, local = part[:i], part[i+1:]
				}
				if i := strings.IndexByte(local, '='); i >= 0 {
					local = local[:i]
				}
				if i := strings.IndexByte(prop, '='); i >= 0 {
					prop = prop[:i]
				}
				prop, local = strings.TrimSpace(prop), strings.TrimSpace(local)
				if !reJSIdent.MatchString(local) || prop == "" {
					continue
				}
				var src *jsSource
				switch {
				case rhs == "process.env":
					src = sc.envSource(prop)
				case strings.HasPrefix(rhs, "context.payload") || strings.HasPrefix(rhs, "github.context.payload"):
					path := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(rhs, "github."), "context.payload"), ".")
					if path != "" {
						path += "."
					}
					src = sc.payloadSource(path + prop)
				}
				if src != nil {
					sc.aliases[local] = src
				}
			}
		}
		if len(sc.aliases) == before {
			break
		}
	}
}

// statementEnd returns the end of the statement that starts at i: the first `;` outside of brackets, or the
// first line break outside of brackets where the statement cannot go on.
func (sc *jsScan) statementEnd(i int) int {
	depth := 0
	for j := i; j < len(sc.clean); j++ {
		if !sc.code(j) {
			continue
		}
		switch sc.clean[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return j
			}
		case ';':
			if depth == 0 {
				return j
			}
		case '\n':
			if depth == 0 && !sc.continues(j) {
				return j
			}
		}
	}
	return len(sc.clean)
}

// continues reports whether the statement goes on after the line break at i: the line ends with an operator or the
// next line starts with one.
func (sc *jsScan) continues(i int) bool {
	prev := sc.prevByte(i)
	if strings.IndexByte("+-*/%=&|?:,.<>(", prev) >= 0 && prev != 0 {
		return true
	}
	for j := i + 1; j < len(sc.clean); j++ {
		c := sc.clean[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return strings.IndexByte("+-*/%&|?:.<>", c) >= 0
	}
	return false
}

var jsPassMethods = map[string]bool{"trim": true, "trimStart": true, "trimEnd": true, "toString": true, "toLowerCase": true, "toUpperCase": true, "slice": true, "substring": true, "substr": true, "normalize": true, "padStart": true, "padEnd": true, "concat": true, "repeat": true}

// sanitized reports whether the read is wrapped in something that may make the value harmless: a call of a function
// (other than String), a method that is not a plain string method, a property. from and to bound the expression.
func (sc *jsScan) sanitized(r jsRef, from, to int) bool {
	// what follows: `.method(...)` or `.property`
	for j := r.end; j < to; {
		for j < to && (sc.clean[j] == ' ' || sc.clean[j] == '\t' || sc.clean[j] == '\n') {
			j++
		}
		if j+1 < to && sc.clean[j] == '?' && sc.clean[j+1] == '.' {
			j++
		}
		if j >= to || sc.clean[j] != '.' {
			break
		}
		j++
		k := j
		for k < to && (sc.clean[k] == '_' || sc.clean[k] == '$' || (sc.clean[k] >= 'a' && sc.clean[k] <= 'z') || (sc.clean[k] >= 'A' && sc.clean[k] <= 'Z') || (sc.clean[k] >= '0' && sc.clean[k] <= '9')) {
			k++
		}
		name := sc.clean[j:k]
		if !(k < to && sc.clean[k] == '(') || !jsPassMethods[name] {
			return true
		}
		j = sc.matchParen(k) + 1
		if j <= k {
			return true
		}
	}
	// what encloses it: a call of a function other than String
	depth := 0
	for j := r.start - 1; j >= from; j-- {
		if sc.cls[j] == jsTplOpen {
			break
		}
		if !sc.code(j) {
			continue
		}
		switch sc.clean[j] {
		case ')':
			depth++
		case '(':
			if depth > 0 {
				depth--
				continue
			}
			k := j
			for k > from && (sc.clean[k-1] == '_' || sc.clean[k-1] == '$' || sc.clean[k-1] == '.' || (sc.clean[k-1] >= 'a' && sc.clean[k-1] <= 'z') || (sc.clean[k-1] >= 'A' && sc.clean[k-1] <= 'Z') || (sc.clean[k-1] >= '0' && sc.clean[k-1] <= '9')) {
				k--
			}
			if callee := sc.clean[k:j]; callee != "" && callee != "String" {
				return true
			}
		}
	}
	return false
}

// matchParen returns the offset of the `)` that closes the `(` at i, or -1.
func (sc *jsScan) matchParen(i int) int {
	depth := 0
	for j := i; j < len(sc.clean); j++ {
		if !sc.code(j) {
			continue
		}
		switch sc.clean[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

var (
	reJSTestAnchored  = regexp.MustCompile(`/\^[^/\n]*\$/[a-z]*\s*\.\s*test\s*\(\s*([^)]+?)\s*\)`)
	reJSMatchAnchored = regexp.MustCompile(`([A-Za-z_$][A-Za-z0-9_$.\[\]'"]*)\s*\.\s*match(?:All)?\s*\(\s*/\^[^/\n]*\$/`)
)

// collectValidated finds the sources that the script tests with an anchored regular expression. Any such test
// counts, wherever it is and whatever it does with the result: a script that checks its input is not reported.
func (sc *jsScan) collectValidated() {
	for _, re := range []*regexp.Regexp{reJSTestAnchored, reJSMatchAnchored} {
		for _, m := range re.FindAllStringSubmatchIndex(sc.clean, -1) {
			for _, r := range sc.refs(m[2], m[3]) {
				sc.validated[r.src.key] = true
			}
		}
	}
}

var (
	reJSCall      = regexp.MustCompile(`(?:([A-Za-z_$][A-Za-z0-9_$]*)\s*\.\s*)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	reJSRequireCP = regexp.MustCompile(`require\(\s*['"](?:node:)?child_process['"]\s*\)\s*\.\s*$`)
	reJSRequire   = regexp.MustCompile(`(?:(?:const|let|var)\s+(\{[^}]*\}|[A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:await\s+)?require\(\s*['"](?:node:)?child_process['"]\s*\)|import\s+(?:\*\s+as\s+([A-Za-z_$][A-Za-z0-9_$]*)|([A-Za-z_$][A-Za-z0-9_$]*)|(\{[^}]*\}))\s+from\s+['"](?:node:)?child_process['"])`)
	reJSPromisfy  = regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:util\.)?promisify\(\s*(?:[A-Za-z_$][A-Za-z0-9_$]*\.)?(exec|execFile)\s*\)`)
	reJSShellOpt  = regexp.MustCompile(`\bshell\s*:\s*(?:true|['"` + "`" + `])`)
	reJSShellArg  = regexp.MustCompile(`^['"](?:ba|z|da|k|a)?sh['"]$`)
	reJSDashC     = regexp.MustCompile(`^['"]-[a-zA-Z]*c['"]$`)
)

// cpImports returns what the script imported from child_process: the names of the namespace objects and the local
// names of the functions.
func (sc *jsScan) cpImports() (spaces map[string]bool, funcs map[string]string) {
	spaces = map[string]bool{"child_process": true, "cp": true, "childProcess": true}
	funcs = map[string]string{}
	addNames := func(list string) {
		for _, part := range strings.Split(strings.Trim(list, "{} \n\t"), ",") {
			name, local := part, part
			if i := strings.IndexAny(part, ":"); i >= 0 {
				name, local = part[:i], part[i+1:]
			} else if i := strings.Index(part, " as "); i >= 0 {
				name, local = part[:i], part[i+4:]
			}
			name, local = strings.TrimSpace(name), strings.TrimSpace(local)
			if name != "" && local != "" {
				funcs[local] = name
			}
		}
	}
	for _, m := range reJSRequire.FindAllStringSubmatch(sc.clean, -1) {
		for _, g := range []string{m[1], m[4]} {
			if strings.HasPrefix(g, "{") {
				addNames(g)
			}
		}
		for _, g := range []string{m[1], m[2], m[3]} {
			if g != "" && !strings.HasPrefix(g, "{") {
				spaces[g] = true
			}
		}
	}
	for _, m := range reJSPromisfy.FindAllStringSubmatch(sc.clean, -1) {
		funcs[m[1]] = m[2]
	}
	return spaces, funcs
}

// jsCPCalls are the functions of child_process that start a program.
var jsCPCalls = map[string]bool{"exec": true, "execSync": true, "execFile": true, "execFileSync": true, "spawn": true, "spawnSync": true}

// findSinks lists the calls that run a shell with untrusted input.
func (sc *jsScan) findSinks() []jsHit {
	spaces, funcs := sc.cpImports()
	var hits []jsHit
	for _, m := range reJSCall.FindAllStringSubmatchIndex(sc.clean, -1) {
		if !sc.code(m[0]) {
			continue
		}
		recv, name := "", sc.clean[m[4]:m[5]]
		if m[2] >= 0 {
			recv = sc.clean[m[2]:m[3]]
		}
		open := m[1] - 1
		for sc.clean[open] != '(' {
			open--
		}
		var api string // "cp" or "actions"
		switch {
		case recv == "exec" && (name == "exec" || name == "getExecOutput"):
			api = "actions"
		case recv != "" && spaces[recv] && jsCPCalls[name]:
			api = "cp"
		case recv == "" && sc.prevByte(m[0]) == '.' && jsCPCalls[name] && reJSRequireCP.MatchString(sc.clean[:m[0]]):
			api = "cp"
		case recv == "" && sc.prevByte(m[0]) != '.':
			if real, ok := funcs[name]; ok && jsCPCalls[real] {
				name, api = real, "cp"
			}
		}
		if api == "" {
			continue
		}
		closeP := sc.matchParen(open)
		if closeP < 0 {
			closeP = len(sc.clean)
		}
		args := sc.splitArgs(open+1, closeP)
		if len(args) == 0 {
			continue
		}
		callee := sc.clean[m[0]:open]
		callee = strings.Join(strings.Fields(callee), "")
		if h, ok := sc.judgeCall(api, name, callee, args, m[0], open, closeP); ok {
			hits = append(hits, h)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].start < hits[j].start })
	return hits
}

// span is a range of the script.
type jsSpan struct{ from, to int }

// splitArgs splits the arguments of a call at the commas outside of brackets and strings.
func (sc *jsScan) splitArgs(from, to int) []jsSpan {
	var out []jsSpan
	depth, start := 0, from
	for j := from; j < to; j++ {
		if sc.cls[j] == jsTplOpen {
			depth++
			j++ // the `{` of `${`
			continue
		}
		if sc.cls[j] == jsTplClose {
			depth--
			continue
		}
		if !sc.code(j) {
			continue
		}
		switch sc.clean[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, jsSpan{start, j})
				start = j + 1
			}
		}
	}
	if strings.TrimSpace(sc.clean[start:to]) != "" {
		out = append(out, jsSpan{start, to})
	}
	return out
}

// untrustedIn returns the first read of an untrusted source in the span that nothing makes harmless.
func (sc *jsScan) untrustedIn(sp jsSpan) (jsRef, bool) {
	for _, r := range sc.refs(sp.from, sp.to) {
		if sc.validated[r.src.key] || sc.sanitized(r, sp.from, sp.to) {
			continue
		}
		return r, true
	}
	return jsRef{}, false
}

// startsWithRef reports whether the span starts with the read: it is the read, or a string or template whose
// first element is the read, or a concatenation that starts with it.
func (sc *jsScan) startsWithRef(sp jsSpan, r jsRef) bool {
	i := sp.from
	for i < sp.to && strings.IndexByte(" \t\n\r`'\"(", sc.clean[i]) >= 0 {
		i++
	}
	if i < sp.to && sc.cls[i] == jsTplOpen {
		i += 2
		for i < sp.to && (sc.clean[i] == ' ') {
			i++
		}
	}
	return i == r.start
}

func (sc *jsScan) judgeCall(api, name, callee string, args []jsSpan, start, open, closeP int) (jsHit, bool) {
	hit := func(r jsRef, kind string) (jsHit, bool) {
		return jsHit{start: start, end: r.end, kind: kind, call: callee, d: r.src.d}, true
	}
	// a shell started with -c: the next element after '-c' holds the script
	if len(args) >= 2 {
		if r, ok := sc.shellCScript(args); ok {
			return hit(r, "shellc")
		}
	}
	if api == "actions" {
		if len(args) == 0 {
			return jsHit{}, false
		}
		if r, ok := sc.untrustedIn(args[0]); ok && sc.startsWithRef(args[0], r) {
			return hit(r, "command")
		}
		return jsHit{}, false
	}
	switch name {
	case "exec", "execSync":
		if r, ok := sc.untrustedIn(args[0]); ok {
			return hit(r, "shell")
		}
	case "spawn", "spawnSync", "execFile", "execFileSync":
		if !reJSShellOpt.MatchString(sc.clean[open+1 : closeP]) {
			return jsHit{}, false
		}
		spans := []jsSpan{args[0]}
		if len(args) > 1 && strings.HasPrefix(strings.TrimSpace(sc.clean[args[1].from:args[1].to]), "[") {
			spans = append(spans, args[1])
		}
		for _, sp := range spans {
			if r, ok := sc.untrustedIn(sp); ok {
				return hit(r, "shell")
			}
		}
	}
	return jsHit{}, false
}

// matchClose returns the offset of the bracket that closes the one at i, or -1.
func (sc *jsScan) matchClose(i int) int {
	depth := 0
	for j := i; j < len(sc.clean); j++ {
		if sc.cls[j] == jsTplOpen {
			depth++
			j++
			continue
		}
		if sc.cls[j] == jsTplClose {
			depth--
			continue
		}
		if !sc.code(j) {
			continue
		}
		switch sc.clean[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// shellCScript finds `'bash', ['-c', script]` (also `'sh', '-c', script`) in the arguments of a call and returns
// the untrusted read in the script.
func (sc *jsScan) shellCScript(args []jsSpan) (jsRef, bool) {
	if !reJSShellArg.MatchString(strings.TrimSpace(sc.clean[args[0].from:args[0].to])) {
		return jsRef{}, false
	}
	items := args[1:]
	if t := strings.TrimSpace(sc.clean[items[0].from:items[0].to]); strings.HasPrefix(t, "[") {
		open := items[0].from + strings.IndexByte(sc.clean[items[0].from:items[0].to], '[')
		if end := sc.matchClose(open); end > open {
			items = sc.splitArgs(open+1, end)
		}
	}
	for i, it := range items {
		if reJSDashC.MatchString(strings.TrimSpace(sc.clean[it.from:it.to])) && i+1 < len(items) {
			return sc.untrustedIn(items[i+1])
		}
	}
	return jsRef{}, false
}
