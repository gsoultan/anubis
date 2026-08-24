// Package yamlmin parses the subset of YAML that a configuration file
// needs, using nothing but the standard library (ADR-0002).
//
// Supported: comments, `key: value` mappings nested by indentation,
// sequences of scalars, and single- or double-quoted strings. Everything
// else YAML can do — anchors, aliases, tags, flow style, multiple documents,
// block scalars — is REJECTED with an error naming the line.
//
// Refusing loudly is the point. A config parser that silently misreads a
// construct it does not really support is worse than one that will not load
// the file at all: the first boots an auth server with settings nobody
// wrote, and the second sends someone to fix line 12.
package yamlmin

import "strconv"

// Kind is the shape of a node.
type Kind int

const (
	// KindScalar is a single value.
	KindScalar Kind = iota
	// KindMap is a mapping, whose key order is preserved.
	KindMap
	// KindSeq is a sequence.
	KindSeq
)

// Node is one parsed value. A document is always a KindMap at the root.
type Node struct {
	Kind Kind
	// Value is set for KindScalar.
	Value string
	// Keys preserves mapping order, so an error can name a field the way the
	// file does and a rewrite can keep the operator's ordering.
	Keys   []string
	Fields map[string]*Node
	// Items is set for KindSeq.
	Items []*Node
	// Line is where this node started, for error messages.
	Line int
}

// Child returns the named field, or nil.
func (n *Node) Child(key string) *Node {
	if n == nil || n.Kind != KindMap {
		return nil
	}
	return n.Fields[key]
}

// At walks a path of mapping keys. A missing step yields nil rather than a
// panic, so callers can ask for optional settings without guarding each hop.
func (n *Node) At(path ...string) *Node {
	cur := n
	for _, key := range path {
		cur = cur.Child(key)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// Str is the scalar at path, or "" when absent or not a scalar.
func (n *Node) Str(path ...string) string {
	got := n.At(path...)
	if got == nil || got.Kind != KindScalar {
		return ""
	}
	return got.Value
}

// Int is the scalar at path parsed as an integer, or def when absent or
// unparseable.
func (n *Node) Int(def int, path ...string) int {
	raw := n.Str(path...)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}

// Bool is the scalar at path as a boolean, or def when absent. YAML's wider
// truthiness (yes/on/y) is accepted because operators write it.
func (n *Node) Bool(def bool, path ...string) bool {
	switch n.Str(path...) {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0":
		return false
	}
	return def
}

// Strings is the sequence at path as scalars. A lone scalar reads as a
// one-element list, since `hosts: one` and `hosts: [one]` mean the same
// thing to whoever wrote it.
func (n *Node) Strings(path ...string) []string {
	got := n.At(path...)
	if got == nil {
		return nil
	}
	if got.Kind == KindScalar {
		return []string{got.Value}
	}
	if got.Kind != KindSeq {
		return nil
	}
	out := make([]string, 0, len(got.Items))
	for _, it := range got.Items {
		if it.Kind == KindScalar {
			out = append(out, it.Value)
		}
	}
	return out
}

// Has reports whether anything exists at path.
func (n *Node) Has(path ...string) bool { return n.At(path...) != nil }
