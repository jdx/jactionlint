//go:build linux

package sandbox

func nums() map[string]int {
	return map[string]int{"io_uring_setup": 425, "bpf": 321, "perf_event_open": 298, "userfaultfd": 323, "keyctl": 250}
}
