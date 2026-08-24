package yamlmin

import (
	"fmt"
	"strings"
)

// maxDepth bounds nesting. A config file is attacker-supplied on any host
// where the config is not solely the operator's, and unbounded recursion on
// indentation is a stack overflow waiting to be reported as a crash.
const maxDepth = 32

// Parse reads a YAML document into a mapping.
//
// An empty document is an empty mapping rather than an error: a config file
// that exists but says nothing is a legitimate "use every default".
func Parse(src []byte) (*Node, error) {
	lines, err := scan(string(src))
	if err != nil {
		return nil, err
	}
	root := &Node{Kind: KindMap, Fields: map[string]*Node{}, Line: 1}
	if len(lines) == 0 {
		return root, nil
	}
	if lines[0].indent != 0 {
		return nil, fmt.Errorf("line %d: document starts indented", lines[0].num)
	}
	rest, err := parseMap(root, lines, 0, 0)
	if err != nil {
		return nil, err
	}
	if rest < len(lines) {
		return nil, fmt.Errorf("line %d: unexpected indentation", lines[rest].num)
	}
	return root, nil
}

// line is one significant line: blanks and comments never reach the parser.
type line struct {
	num    int
	indent int
	text   string
}

func scan(src string) ([]line, error) {
	var out []line
	for i, raw := range strings.Split(src, "\n") {
		num := i + 1
		text := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(text) == "" || strings.HasPrefix(strings.TrimSpace(text), "#") {
			continue
		}
		indent := 0
		for indent < len(text) && text[indent] == ' ' {
			indent++
		}
		if indent < len(text) && text[indent] == '\t' {
			return nil, fmt.Errorf("line %d: tab used for indentation (YAML forbids it; use spaces)", num)
		}
		body := text[indent:]
		if strings.HasPrefix(body, "---") || strings.HasPrefix(body, "...") {
			return nil, fmt.Errorf("line %d: multiple documents are not supported", num)
		}
		out = append(out, line{num: num, indent: indent, text: body})
	}
	return out, nil
}

// parseMap fills dst with every entry at exactly `indent`, returning the
// index of the first line that belongs to an outer level.
func parseMap(dst *Node, lines []line, i, indent int) (int, error) {
	if indent > maxDepth {
		return 0, fmt.Errorf("line %d: nested deeper than %d levels", lines[i].num, maxDepth)
	}
	for i < len(lines) {
		cur := lines[i]
		if cur.indent < indent {
			return i, nil
		}
		if cur.indent > indent {
			return 0, fmt.Errorf("line %d: unexpected indentation", cur.num)
		}
		if strings.HasPrefix(cur.text, "- ") || cur.text == "-" {
			return 0, fmt.Errorf("line %d: list item where a `key: value` was expected", cur.num)
		}
		key, rest, err := splitKey(cur)
		if err != nil {
			return 0, err
		}
		if _, dup := dst.Fields[key]; dup {
			// Silently keeping one of them means half the readers of this
			// file are wrong about what the server loaded.
			return 0, fmt.Errorf("line %d: duplicate key %q", cur.num, key)
		}

		if rest != "" {
			val, err := scalar(rest, cur.num)
			if err != nil {
				return 0, err
			}
			dst.set(key, &Node{Kind: KindScalar, Value: val, Line: cur.num})
			i++
			continue
		}

		// A bare `key:` owns whatever is indented beneath it.
		child, next, err := parseBlock(lines, i+1, indent, cur.num)
		if err != nil {
			return 0, err
		}
		dst.set(key, child)
		i = next
	}
	return i, nil
}

// parseBlock reads the block beneath a bare `key:` — a nested mapping, a
// sequence, or nothing at all (which is an empty mapping, not an error).
func parseBlock(lines []line, i, parentIndent, keyLine int) (*Node, int, error) {
	if i >= len(lines) || lines[i].indent <= parentIndent {
		return &Node{Kind: KindMap, Fields: map[string]*Node{}, Line: keyLine}, i, nil
	}
	indent := lines[i].indent
	if strings.HasPrefix(lines[i].text, "- ") || lines[i].text == "-" {
		seq := &Node{Kind: KindSeq, Line: lines[i].num}
		for i < len(lines) && lines[i].indent == indent {
			text := lines[i].text
			if !strings.HasPrefix(text, "- ") && text != "-" {
				break
			}
			item := strings.TrimSpace(strings.TrimPrefix(text, "-"))
			if item == "" {
				return nil, 0, fmt.Errorf("line %d: only sequences of values are supported", lines[i].num)
			}
			val, err := scalar(item, lines[i].num)
			if err != nil {
				return nil, 0, err
			}
			seq.Items = append(seq.Items, &Node{Kind: KindScalar, Value: val, Line: lines[i].num})
			i++
		}
		if i < len(lines) && lines[i].indent > parentIndent && lines[i].indent != indent {
			return nil, 0, fmt.Errorf("line %d: unexpected indentation", lines[i].num)
		}
		return seq, i, nil
	}
	m := &Node{Kind: KindMap, Fields: map[string]*Node{}, Line: lines[i].num}
	next, err := parseMap(m, lines, i, indent)
	if err != nil {
		return nil, 0, err
	}
	return m, next, nil
}

func (n *Node) set(key string, child *Node) {
	if n.Fields == nil {
		n.Fields = map[string]*Node{}
	}
	n.Keys = append(n.Keys, key)
	n.Fields[key] = child
}

// splitKey separates `key:` from the rest of the line, honouring quotes so a
// colon inside a value does not look like a second key.
func splitKey(l line) (string, string, error) {
	text := l.text
	if strings.HasPrefix(text, "\"") || strings.HasPrefix(text, "'") {
		return "", "", fmt.Errorf("line %d: quoted keys are not supported", l.num)
	}
	idx := strings.IndexByte(text, ':')
	if idx < 0 {
		return "", "", fmt.Errorf("line %d: expected `key: value`", l.num)
	}
	key := strings.TrimSpace(text[:idx])
	if key == "" {
		return "", "", fmt.Errorf("line %d: empty key", l.num)
	}
	rest := strings.TrimSpace(text[idx+1:])
	return key, rest, nil
}

// scalar reads one value, rejecting the YAML constructs this parser does not
// implement rather than misreading them.
func scalar(raw string, num int) (string, error) {
	switch raw[0] {
	case '"':
		return quoted(raw, num)
	case '\'':
		return singleQuoted(raw, num)
	case '&', '*':
		return "", fmt.Errorf("line %d: anchors and aliases are not supported", num)
	case '!':
		return "", fmt.Errorf("line %d: tags are not supported", num)
	case '{', '[':
		return "", fmt.Errorf("line %d: flow style is not supported; use indented blocks", num)
	case '|', '>':
		return "", fmt.Errorf("line %d: block scalars are not supported", num)
	}
	// An unquoted value ends at a trailing comment, which must be preceded
	// by a space — otherwise a password containing '#' would be truncated.
	if idx := strings.Index(raw, " #"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw), nil
}

func quoted(raw string, num int) (string, error) {
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		if c == '\\' {
			if i+1 >= len(raw) {
				return "", fmt.Errorf("line %d: string ends with a dangling escape", num)
			}
			i++
			switch raw[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"', '\\', '/':
				b.WriteByte(raw[i])
			default:
				return "", fmt.Errorf("line %d: unknown escape \\%c", num, raw[i])
			}
			continue
		}
		if c == '"' {
			return b.String(), nil
		}
		b.WriteByte(c)
	}
	return "", fmt.Errorf("line %d: unterminated string", num)
}

// singleQuoted is literal apart from ” meaning one quote, as YAML defines.
func singleQuoted(raw string, num int) (string, error) {
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		if raw[i] == '\'' {
			if i+1 < len(raw) && raw[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			return b.String(), nil
		}
		b.WriteByte(raw[i])
	}
	return "", fmt.Errorf("line %d: unterminated string", num)
}
