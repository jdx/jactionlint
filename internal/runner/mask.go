package runner

import (
	"sort"
	"strings"
	"sync"
)

// Masker replaces secret values in the output of the steps with ***.
type Masker struct {
	mu   sync.RWMutex
	vals []string // longest first
}

// Add registers a value to hide. Each line of a multi-line value is hidden on its own, because output is
// filtered by line. Empty values and a lone whitespace are ignored.
func (m *Masker) Add(v string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, line := range strings.Split(strings.ReplaceAll(v, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		dup := false
		for _, e := range m.vals {
			if e == line {
				dup = true
				break
			}
		}
		if !dup {
			m.vals = append(m.vals, line)
		}
	}
	sort.SliceStable(m.vals, func(i, j int) bool { return len(m.vals[i]) > len(m.vals[j]) })
}

// Mask returns s with every registered value replaced.
func (m *Masker) Mask(s string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.vals {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, "***")
		}
	}
	return s
}
