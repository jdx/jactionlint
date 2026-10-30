//go:build linux

package sandbox

func nums() map[string]int {
	return map[string]int{"io_uring_setup": 425, "bpf": 280, "perf_event_open": 241, "userfaultfd": 282, "keyctl": 219}
}
