package jactionlint

import "testing"

// The regression suite of the reader of PowerShell scripts that decides whether an azure/powershell step runs the workspace
// (see the supported subset in the checks document).
func TestPowerShellRunsWorkspace(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   bool
	}{
		// commands that are paths
		{"relative path", "./build.ps1", true},
		{"relative path with backslash", `.\build.ps1`, true},
		{"parent directory", "../tools/build.ps1", true},
		{"path with a directory", "scripts/deploy.ps1", true},
		{"path with a directory and backslashes", `scripts\deploy.ps1`, true},
		{"bare name", "build.ps1", false},
		{"workspace variable", `$env:GITHUB_WORKSPACE\build.ps1`, true},
		{"workspace variable in braces", `${env:GITHUB_WORKSPACE}/build.ps1`, true},
		{"workspace expression", "${{ github.workspace }}/build.ps1", true},
		{"workspace expression without spaces", "${{github.workspace}}/build.ps1", true},
		{"absolute path", `C:\tools\build.ps1`, false},
		{"root path", "/usr/bin/tool", false},
		{"url", "https://example.com/install.ps1", false},
		{"module-qualified cmdlet", `Microsoft.PowerShell.Management\Get-Item C:\x`, false},

		// operators
		{"call operator", "& ./build.ps1 -a b", true},
		{"call operator with a quoted path", `& '.\build.ps1'`, true},
		{"call operator with a variable", "& $script", false},
		{"dot-sourcing", ". ./lib.ps1", true},
		{"dot-sourcing with a quoted path", `. "./lib.ps1"`, true},
		{"quoted path alone is a value", `"./build.ps1"`, false},

		// assignments
		{"assignment of a script", "$r = ./build.ps1", true},
		{"assignment without spaces", "$r=./build.ps1", true},
		{"assignment with a type", "[string]$r = ./build.ps1", true},
		{"assignment with an addition", "$r += ./build.ps1", true},
		{"assignment of a call", "$r = & ./build.ps1", true},
		{"assignment of a dot-sourced script", "$r = . ./build.ps1", true},
		{"assignment of a build", "$r = npm ci", true},
		{"assignment of a division", "$x = $a / $b", false},
		{"assignment of a division of numbers", "$x = 10 / 2", false},
		{"assignment of a method", `$x = [System.IO.Path]::Combine("a", "b")`, false},
		{"assignment of a module-qualified cmdlet", `$r = Microsoft.PowerShell.Management\Get-Item C:\x`, false},
		{"assignment of the content of a file", "$x = Get-Content ./build.ps1", false},
		{"assignment of a quoted runner", "$x = 'npm'", false},
		{"assignment of a quoted runner in double quotes", `$x = "go"`, false},
		{"assignment of a url", "$x = Invoke-RestMethod https://example.com/a/b.ps1", false},
		{"assignment of a path as text", `$x = "./build.ps1"`, false},

		// runners and interpreters
		{"build tool", "npm ci", true},
		{"build tool with an extension", "dotnet.exe build", true},
		{"quoted build tool", "'npm' ci", false},
		{"quoted build tool after the call operator", "& 'npm' ci", true},
		{"interpreter with a script", "python scripts/x.py", true},
		{"interpreter with a bare script", "python x.py", false},
		{"pwsh with a file", "pwsh -NoProfile -File ./x.ps1", true},
		{"pwsh with a file below a directory", "pwsh -File tools/x.ps1", true},
		{"pwsh with a command", `pwsh -Command "& ./x.ps1"`, true},
		{"pwsh with a command that does not run a file", `pwsh -Command "Get-Date"`, false},
		{"import of a module", "Import-Module ./m.psm1", true},
		{"import of an installed module", "Import-Module Az.Accounts", false},

		// pipelines, blocks, separators
		{"pipeline start", "./build.ps1 | Out-Null", true},
		{"pipeline stage", "Get-Date | ./filter.ps1", true},
		{"after a semicolon", "Write-Host a; ./build.ps1", true},
		{"after a conjunction", "Write-Host a && ./build.ps1", true},
		{"in an if block", "if ($true) { ./build.ps1 }", true},
		{"in a foreach block", "foreach ($f in 1..3) {\n  & ./build.ps1 $f\n}", true},
		{"in a subexpression", "$x = $(./build.ps1)", true},
		{"in a parenthesis", "$x = (./build.ps1)", true},

		// text that is not a command
		{"argument", "Get-Content ./build.ps1", false},
		{"argument with a directory", "Get-Content scripts/x.ps1", false},
		{"copy", "Copy-Item a/b c/d", false},
		{"saving an installer", "Invoke-WebRequest https://example.com/install.ps1 -OutFile install.ps1", false},
		{"string", `Write-Host "run ./build.ps1"`, false},
		{"line comment", "# ./build.ps1\nGet-Date", false},
		{"comment after a command", "Get-Date # ./build.ps1", false},
		{"block comment", "<# ./build.ps1 #>\nGet-Date", false},
		{"here-string", "$x = @'\n./build.ps1\n'@\nGet-Date", false},
		{"here-string in double quotes", "$x = @\"\n./build.ps1\n\"@\nGet-Date", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := powerShellRunsWorkspace(tc.script); got != tc.want {
				t.Errorf("powerShellRunsWorkspace(%q) = %v, want %v", tc.script, got, tc.want)
			}
		})
	}
}
