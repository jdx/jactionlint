package runner

import (
	"bytes"
	"fmt"
	"strings"
)

// command is a workflow command, `::name key=value,key=value::message`, found on a line of a step's output.
type command struct {
	Name    string
	Props   map[string]string
	Message string
}

var (
	messageUnescape  = strings.NewReplacer("%0D", "\r", "%0A", "\n", "%25", "%")
	propertyUnescape = strings.NewReplacer("%0D", "\r", "%0A", "\n", "%3A", ":", "%2C", ",", "%25", "%")
)

// parseCommand parses a line as a workflow command. ok is false when the line is not one.
func parseCommand(line string) (c command, ok bool) {
	if !strings.HasPrefix(line, "::") {
		return c, false
	}
	rest := line[2:]
	end := strings.Index(rest, "::")
	if end < 0 {
		return c, false
	}
	head, msg := rest[:end], rest[end+2:]
	name, props, _ := strings.Cut(head, " ")
	if name == "" || strings.ContainsAny(name, " \t") {
		return c, false
	}
	c.Name = strings.ToLower(name)
	c.Message = messageUnescape.Replace(msg)
	c.Props = map[string]string{}
	for _, kv := range strings.Split(props, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		c.Props[strings.ToLower(strings.TrimSpace(k))] = propertyUnescape.Replace(v)
	}
	return c, true
}

// lineSink turns a byte stream into lines and hands each to handle, without the line ending. Close flushes a last
// line without one.
type lineSink struct {
	buf    bytes.Buffer
	handle func(line string)
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.buf.Write(p)
	for {
		b := s.buf.Bytes()
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			if s.buf.Len() > 1<<20 { // a line without end: do not hold the whole output
				s.handle(s.buf.String())
				s.buf.Reset()
			}
			return len(p), nil
		}
		line := strings.TrimSuffix(string(b[:i]), "\r")
		s.buf.Next(i + 1)
		s.handle(line)
	}
}

func (s *lineSink) Close() {
	if s.buf.Len() > 0 {
		s.handle(strings.TrimSuffix(s.buf.String(), "\r"))
		s.buf.Reset()
	}
}

// annotation is an ::error::, ::warning:: or ::notice:: of a step.
type annotation struct {
	Level string
	Title string
	File  string
	Line  string
	Col   string
	Msg   string
}

func (a annotation) String() string {
	loc := ""
	if a.File != "" {
		loc = a.File
		if a.Line != "" {
			loc += ":" + a.Line
			if a.Col != "" {
				loc += ":" + a.Col
			}
		}
		loc += ": "
	}
	title := ""
	if a.Title != "" {
		title = a.Title + ": "
	}
	return fmt.Sprintf("%s: %s%s%s", a.Level, loc, title, a.Msg)
}
