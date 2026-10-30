//go:build !linux && !darwin

package runner

import "errors"

func cloneFile(src, dst string) error { return errors.New("no clone support") }
