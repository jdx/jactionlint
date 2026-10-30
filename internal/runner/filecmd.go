package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// maxFileCommand bounds what the runner reads from a file a step wrote: the step is not trusted.
const maxFileCommand = 8 << 20

// readFileCommand reads a file that a step could write to, without following a symlink the step put in its place.
func readFileCommand(path string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|noFollow, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("cannot read %s (a step replaced it?): %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxFileCommand+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxFileCommand {
		return "", fmt.Errorf("%s is larger than %d bytes", path, maxFileCommand)
	}
	return string(b), nil
}

// parseKeyValues parses the format of GITHUB_ENV, GITHUB_OUTPUT and GITHUB_STATE: `name=value` lines and
// `name<<DELIMITER` blocks that end with a line holding DELIMITER. Later entries win and the order is kept.
func parseKeyValues(text string) (keys []string, vals map[string]string, err error) {
	vals = map[string]string{}
	set := func(k, v string) {
		if _, ok := vals[k]; !ok {
			keys = append(keys, k)
		}
		vals[k] = v
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			continue
		}
		eq := strings.Index(line, "=")
		hd := strings.Index(line, "<<")
		if hd >= 0 && (eq < 0 || hd < eq) {
			key, delim := line[:hd], line[hd+2:]
			if key == "" || delim == "" {
				return keys, vals, fmt.Errorf("invalid format %q", line)
			}
			var body []string
			j := i + 1
			for ; j < len(lines); j++ {
				if lines[j] == delim {
					break
				}
				body = append(body, lines[j])
			}
			if j >= len(lines) {
				return keys, vals, fmt.Errorf("matching delimiter %q not found for %q", delim, key)
			}
			set(key, strings.Join(body, "\n"))
			i = j
			continue
		}
		if eq < 1 {
			return keys, vals, fmt.Errorf("invalid format %q", line)
		}
		set(line[:eq], line[eq+1:])
	}
	return keys, vals, nil
}
