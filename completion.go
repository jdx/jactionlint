package jactionlint

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// This file generates the completion scripts of --completion=SHELL. The options come from the table cliOptions
// (so a new option is completed without touching this file), and the values from the sets which the options
// accept: the profiles, the formats, the rule IDs of the registry and so on. The script of a binary completes
// the rule IDs of that binary: generate it again after an upgrade.

// completionShells are the values of --completion.
var completionShells = []string{"bash", "zsh", "fish", "powershell"}

// compOption is one option as the completion scripts see it.
type compOption struct {
	long, short string
	arg         string // the name of the value, "" for a flag
	optional    bool   // the value can only follow "="
	help        string
	choices     []string // the values the option accepts, if they are a fixed set
	rules       int      // 1: a rule ID, 2: a comma separated list of rule IDs
	file        bool     // the value is a file
}

const (
	compRuleID   = 1
	compRuleList = 2
)

// completionOptions lists the options with the values to complete.
func completionOptions() []compOption {
	choices := map[string][]string{
		"format":       slices.Clone(nativeFormats),
		"profile":      {string(ProfileCorrectness), string(ProfileDefault), string(ProfilePedantic)},
		"min-severity": {"info", "warn", "error"},
		"color":        {"always", "never", "auto"},
		"fix":          {"unsafe"},
		"online":       {string(OnlineModeCache), string(OnlineModeStrict), string(OnlineModeCacheStrict)},
		"completion":   completionShells,
	}
	rules := map[string]int{"ignore": compRuleID, "explain": compRuleID, "fix-rules": compRuleList, "select": compRuleList}
	var ret []compOption
	for _, o := range cliOptions {
		c := compOption{long: o.Long, short: o.Short, arg: o.ArgName, optional: o.Optional, help: o.Help, choices: choices[o.Long], rules: rules[o.Long]}
		switch o.ArgName {
		case "FILE":
			c.file = true
		}
		ret = append(ret, c)
	}
	return ret
}

// compWords returns the words which a single option offers, as the scripts list them.
func (o compOption) values(ids []string) []string {
	if o.rules != 0 {
		return ids
	}
	return o.choices
}

// writeCompletion writes the completion script of the shell.
func writeCompletion(w io.Writer, shell string) error {
	opts := completionOptions()
	ids := make([]string, 0, len(ruleRegistry))
	for _, r := range ruleRegistry {
		ids = append(ids, r.ID)
	}
	switch shell {
	case "bash":
		return writeBashCompletion(w, opts, ids)
	case "zsh":
		return writeZshCompletion(w, opts, ids)
	case "fish":
		return writeFishCompletion(w, opts, ids)
	case "powershell", "pwsh":
		return writePowerShellCompletion(w, opts, ids)
	}
	return &usageError{fmt.Errorf("invalid value %q for --completion. available values are %s", shell, quotes(completionShells))}
}

func flagsOf(opts []compOption) (long, withValue []string) {
	for _, o := range opts {
		l := "--" + o.long
		if o.arg != "" && !o.optional {
			withValue = append(withValue, l)
		}
		if o.optional {
			l += "="
		}
		long = append(long, l)
		if o.short != "" {
			long = append(long, "-"+o.short)
			if o.arg != "" && !o.optional {
				withValue = append(withValue, "-"+o.short)
			}
		}
	}
	return long, withValue
}

// --- bash --------------------------------------------------------------------------------------------------

func writeBashCompletion(w io.Writer, opts []compOption, ids []string) error {
	long, withValue := flagsOf(opts)
	var b strings.Builder
	b.WriteString("# bash completion for jactionlint. Load it with:  source <(jactionlint --completion=bash)\n")
	b.WriteString("_jactionlint() {\n")
	b.WriteString("    local cur=\"${COMP_WORDS[COMP_CWORD]}\" prev=\"\" opt=\"\" val=\"\" pre=\"\" i\n")
	b.WriteString("    COMPREPLY=()\n")
	b.WriteString("    # Bash splits --opt=value at the \"=\": rebuild the option and the value\n")
	b.WriteString("    if [[ $cur == \"=\" ]]; then\n        opt=${COMP_WORDS[COMP_CWORD-1]}; val=\n")
	b.WriteString("    elif [[ $COMP_CWORD -ge 2 && ${COMP_WORDS[COMP_CWORD-1]} == \"=\" ]]; then\n        opt=${COMP_WORDS[COMP_CWORD-2]}; val=$cur\n")
	b.WriteString("    elif [[ $COMP_CWORD -ge 1 ]]; then\n        prev=${COMP_WORDS[COMP_CWORD-1]}; val=$cur\n    fi\n")
	fmt.Fprintf(&b, "    local flags=%s\n", bashQuote(strings.Join(long, " ")))
	fmt.Fprintf(&b, "    local withvalue=\" %s \"\n", strings.Join(withValue, " "))
	fmt.Fprintf(&b, "    local rules=%s\n", bashQuote(strings.Join(ids, " ")))
	b.WriteString("    # The option whose value is being completed: after \"=\", or the previous word of an option which takes a value\n")
	b.WriteString("    if [[ -z $opt && $withvalue == *\" $prev \"* ]]; then opt=$prev; fi\n")
	b.WriteString("    if [[ -n $opt ]]; then\n")
	b.WriteString("        case $opt in\n")
	for _, o := range opts {
		names := "--" + o.long
		if o.short != "" {
			names += "|-" + o.short
		}
		switch {
		case o.rules == compRuleList:
			// In the format string "%%" is a literal "%": the script has ${val%,*} (up to the last comma), which keeps
			// all the elements before the one being completed
			fmt.Fprintf(&b, "            %s)\n                pre=${val%%,*}; [[ $val == *,* ]] && pre=${val%%,*}, || pre=\n                COMPREPLY=($(compgen -P \"$pre\" -W \"$rules\" -- \"${val##*,}\"))\n                compopt -o nospace 2>/dev/null; return ;;\n", names)
		case o.rules == compRuleID:
			fmt.Fprintf(&b, "            %s) COMPREPLY=($(compgen -W \"$rules\" -- \"$val\")); return ;;\n", names)
		case len(o.choices) > 0:
			fmt.Fprintf(&b, "            %s) COMPREPLY=($(compgen -W %s -- \"$val\")); return ;;\n", names, bashQuote(strings.Join(o.choices, " ")))
		case o.file:
			fmt.Fprintf(&b, "            %s) COMPREPLY=($(compgen -f -- \"$val\")); return ;;\n", names)
		case o.arg != "":
			fmt.Fprintf(&b, "            %s) return ;;\n", names)
		}
	}
	b.WriteString("        esac\n    fi\n")
	b.WriteString("    if [[ $cur == -* ]]; then\n        COMPREPLY=($(compgen -W \"$flags\" -- \"$cur\"))\n        [[ ${COMPREPLY[0]} == *= ]] && compopt -o nospace 2>/dev/null\n        return\n    fi\n")
	b.WriteString("    COMPREPLY=($(compgen -f -X '!*.@(yml|yaml)' -- \"$cur\") $(compgen -d -S / -- \"$cur\"))\n")
	b.WriteString("}\n")
	b.WriteString("complete -o filenames -F _jactionlint jactionlint\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func bashQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// --- zsh ---------------------------------------------------------------------------------------------------

func zshEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `'\''`, `[`, `\[`, `]`, `\]`, `:`, `\:`)
	return r.Replace(s)
}

func writeZshCompletion(w io.Writer, opts []compOption, ids []string) error {
	var b strings.Builder
	b.WriteString("#compdef jactionlint\n# zsh completion for jactionlint. Put it in a directory of $fpath as _jactionlint, or load it with:\n#   source <(jactionlint --completion=zsh)\n\n")
	b.WriteString("_jactionlint() {\n")
	fmt.Fprintf(&b, "    local -a rules\n    rules=(%s)\n", strings.Join(ids, " "))
	b.WriteString("    _arguments -s -S \\\n")
	for _, o := range opts {
		spec := zshOptionSpec(o)
		b.WriteString("        " + spec + " \\\n")
	}
	b.WriteString("        '*:workflow or action file:_files -g \"*.(yml|yaml)\"'\n")
	b.WriteString("}\n\n")
	b.WriteString("if [ \"$funcstack[1]\" = \"_jactionlint\" ]; then\n    _jactionlint \"$@\"\nelse\n    compdef _jactionlint jactionlint\nfi\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func zshOptionSpec(o compOption) string {
	help := zshEscape(o.help)
	names := []string{"--" + o.long}
	if o.short != "" {
		names = append(names, "-"+o.short)
	}
	var action string
	switch {
	case o.arg == "":
	case o.rules == compRuleList:
		action = fmt.Sprintf(":%s:{_values -s , \"rule ID\" $rules}", zshEscape(strings.ToLower(o.arg)))
	case o.rules == compRuleID:
		action = fmt.Sprintf(":%s:{compadd -a rules}", zshEscape(strings.ToLower(o.arg)))
	case len(o.choices) > 0:
		action = fmt.Sprintf(":%s:(%s)", zshEscape(strings.ToLower(o.arg)), strings.Join(zshWords(o.choices), " "))
	case o.file:
		action = fmt.Sprintf(":%s:_files", zshEscape(strings.ToLower(o.arg)))
	default:
		action = fmt.Sprintf(":%s:", zshEscape(strings.ToLower(o.arg)))
	}
	exclusive := ""
	if o.long == "help" || o.long == "version" || o.long == "completion" {
		exclusive = "(- *)"
	}
	if o.optional {
		// an optional value can only be given with "="
		var parts []string
		for _, n := range names {
			parts = append(parts, fmt.Sprintf("'%s%s=-[%s]%s'", exclusive, n, help, action))
		}
		return strings.Join(parts, " \\\n        ")
	}
	var parts []string
	for _, n := range names {
		// "--opt=" takes the value after "=" or as the next word, "-o+" attached or as the next word
		s := "="
		if o.arg == "" {
			s = ""
		} else if !strings.HasPrefix(n, "--") {
			s = "+"
		}
		parts = append(parts, fmt.Sprintf("'%s%s%s[%s]%s'", exclusive, n, s, help, action))
	}
	return strings.Join(parts, " \\\n        ")
}

// zshWords quotes the values of an _arguments list: a value with a comma or a space is written with quotes.
func zshWords(ss []string) []string {
	ret := make([]string, len(ss))
	for i, s := range ss {
		if strings.ContainsAny(s, " ,") {
			s = `\"` + s + `\"`
		}
		ret[i] = s
	}
	return ret
}

// --- fish --------------------------------------------------------------------------------------------------

func fishEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}

func writeFishCompletion(w io.Writer, opts []compOption, ids []string) error {
	var b strings.Builder
	b.WriteString("# fish completion for jactionlint. Load it with:  jactionlint --completion=fish | source\n")
	b.WriteString("# or save it as ~/.config/fish/completions/jactionlint.fish\n")
	b.WriteString("complete -c jactionlint -f -a '(__fish_complete_suffix .yml .yaml)'\n")
	for _, o := range opts {
		line := "complete -c jactionlint -l " + o.long
		if o.short != "" {
			line += " -s " + o.short
		}
		vals := o.values(ids)
		switch {
		case o.arg == "":
		case o.optional:
			// the value follows "=": there is no -r, fish offers the values after "--opt="
		case len(vals) > 0:
			line += " -x"
		case o.file:
			line += " -r -F"
		default:
			line += " -x"
		}
		if len(vals) > 0 {
			line += " -a '" + fishEscape(strings.Join(vals, " ")) + "'"
		}
		line += " -d '" + fishEscape(o.help) + "'"
		b.WriteString(line + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// --- PowerShell --------------------------------------------------------------------------------------------

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func writePowerShellCompletion(w io.Writer, opts []compOption, ids []string) error {
	var b strings.Builder
	b.WriteString("# PowerShell completion for jactionlint. Load it with:  jactionlint --completion=powershell | Out-String | Invoke-Expression\n")
	b.WriteString("Register-ArgumentCompleter -Native -CommandName jactionlint -ScriptBlock {\n")
	b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
	var psOpts []string
	for _, o := range opts {
		psOpts = append(psOpts, fmt.Sprintf("@{ Name = %s; Short = %s; Arg = %s; Optional = %s; File = %s; List = %s; Help = %s }",
			psQuote("--"+o.long), psQuote(shortName(o)), psQuote(o.arg), psBool(o.optional), psBool(o.file), psBool(o.rules == compRuleList), psQuote(o.help)))
	}
	b.WriteString("    $options = @(\n        " + strings.Join(psOpts, ",\n        ") + "\n    )\n")
	b.WriteString("    $values = @{\n")
	for _, o := range opts {
		vals := o.values(ids)
		if len(vals) == 0 {
			continue
		}
		quoted := make([]string, len(vals))
		for i, v := range vals {
			quoted[i] = psQuote(v)
		}
		fmt.Fprintf(&b, "        %s = @(%s)\n", psQuote("--"+o.long), strings.Join(quoted, ", "))
	}
	b.WriteString("    }\n")
	b.WriteString(`    $words = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })
    $prev = if ($wordToComplete -eq '') { $words[-1] } elseif ($words.Count -ge 2) { $words[-2] } else { '' }
    $o = $null; $prefix = ''; $word = $wordToComplete
    if ($word -match '^(--[^=]+)=(.*)$') {
        $o = $options | Where-Object { $_.Name -eq $Matches[1] -and $_.Arg -ne '' } | Select-Object -First 1
        if ($o) { $prefix = $Matches[1] + '='; $word = $Matches[2] }
    } elseif ($words.Count -ge 2 -and $words[-1] -match '^(--[^=]+)=.*,') {
        # --option=a,b,c: PowerShell reads a list and completes its last element only, which replaces just that element
        $o = $options | Where-Object { $_.Name -eq $Matches[1] -and $_.Arg -ne '' } | Select-Object -First 1
    } elseif ($prev -like '-*') {
        # A value which is optional follows only "=": after a space it is a file name, not "--color always"
        $o = $options | Where-Object { ($_.Name -eq $prev -or $_.Short -eq $prev) -and $_.Arg -ne '' -and -not $_.Optional } | Select-Object -First 1
    }
    if ($o) {
        # A comma separated list: complete its last element and keep the ones before it
        $head = ''
        if ($o.List -and $word.Contains(',')) { $i = $word.LastIndexOf(','); $head = $word.Substring(0, $i + 1); $word = $word.Substring($i + 1) }
        if ($values.ContainsKey($o.Name)) {
            $values[$o.Name] | Where-Object { $_ -like "$word*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($prefix + $head + $_, $_, 'ParameterValue', $_)
            }
        } elseif ($o.File -and $prefix -ne '') {
            # After "--option=" PowerShell does not complete file names itself. Elsewhere returning nothing does
            [System.Management.Automation.CompletionCompleters]::CompleteFilename($word) | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($prefix + $_.CompletionText, $_.ListItemText, $_.ResultType, $_.ToolTip)
            }
        }
        return
    }
    if ($wordToComplete -like '-*') {
        $options | ForEach-Object {
            if ($_.Name -like "$wordToComplete*") { [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ParameterName', $_.Help) }
            if ($_.Short -and $_.Short -like "$wordToComplete*") { [System.Management.Automation.CompletionResult]::new($_.Short, $_.Short, 'ParameterName', $_.Help) }
        }
    }
    # Anything else (the files to lint) falls back to the file name completion of PowerShell
}
`)
	_, err := io.WriteString(w, b.String())
	return err
}

func psBool(b bool) string {
	if b {
		return "$true"
	}
	return "$false"
}

func shortName(o compOption) string {
	if o.short == "" {
		return ""
	}
	return "-" + o.short
}
