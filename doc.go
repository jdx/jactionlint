/*
Package jactionlint is the implementation of jactionlint linter. It's a static checker for GitHub
Actions workflow files.

https://github.com/jdx/jactionlint

jactionlint is a command line tool but it also provides Go API for Go programs. It includes a
workflow file parser built on top of yaml/go-yaml, lexer/parser/checker for expressions embedded by
${{ }} placeholder, popular actions data, available contexts information, etc.

To run the linter, Linter is the struct which manages the entire linter lifecycle. Please see the
first example.

jactionlint also provides the flexibility to add your own rules by implementing Rule interface.
Please read the YourOwnRule example.

# Library versioning

The version is for the command line tool. So it does not represent the version of the library. It
means that the library does not follow semantic versioning and any patch version bump may introduce
some breaking changes.

# Go version compatibility

Minimum supported Go version is written in go.mod file in this library. That said, older Go versions
are actually not tested on CI. Last two major Go versions are recommended because they're tested on
CI. For example, when the latest Go version is v1.22, v1.21 and v1.22 are nice to use.

https://github.com/jdx/jactionlint/blob/main/go.mod

# Other documentations

All documentations for jactionlint can be found in the following page.

https://github.com/jdx/jactionlint/tree/main/docs

# License

This library is provided under the MIT license.

> Copyright (c) 2021 rhysd
>
> Copyright (c) 2026 jdx

Full text can be found in the following page.

https://github.com/jdx/jactionlint/blob/main/LICENSE.txt
*/
package jactionlint
