//go:build unix

package runner

import "syscall"

const noFollow = syscall.O_NOFOLLOW
