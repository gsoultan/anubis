package scopedomain

import (
	"encoding/base64"
	"errors"
	"strings"
)

type ScopeNodeRecord struct {
	ID          string
	Axis        string
	NodeType    string
	ParentID    string
	Slug        string
	Name        string
	ExternalRef string
	Status      string
	IsAxisRoot  bool
}

// DefaultScopeNodePage / MaxScopeNodePage bound one page of a node listing.
// The old query had a hard LIMIT 2000 and no cursor, so "list this axis" was
// silently a prefix — including for the sync archive pass, which decided what
// to archive from that prefix.
//
// The default deliberately MATCHES that old limit. Existing callers send no
// page_size, and lowering the default under them would turn "you got the
// first 2000" into "you got the first N" — a smaller silent truncation is
// still a silent truncation, and the console reads this endpoint to render
// tree levels. Paging is now available to anyone who asks for it; nobody's
// first page got shorter.
const (
	DefaultScopeNodePage int32 = 2000
	MaxScopeNodePage     int32 = 2000
)

// ScopeNodeFilter selects one page of scope nodes.
//
// Paging is KEYSET: the listing is ordered by (name, id) and a page resumes
// strictly after the last row of the previous one. name is not unique, so id
// is the tiebreaker — without it a page boundary landing inside a run of
// equal names would skip or repeat rows.
type ScopeNodeFilter struct {
	Axis            string
	ParentID        string // empty = the whole axis
	Query           string
	IncludeArchived bool
	// AfterName/AfterID resume strictly after this position. Empty AfterName
	// starts at the beginning.
	AfterName string
	AfterID   string
	// Limit is clamped to MaxScopeNodePage; zero means DefaultScopeNodePage.
	Limit int32
}

// Normalise clamps Limit into range. Callers pass user input straight in, so
// this is what stops a page size of 0 returning nothing and a page size of
// 10^9 returning the whole axis.
func (f ScopeNodeFilter) Normalise() ScopeNodeFilter {
	if f.Limit <= 0 {
		f.Limit = DefaultScopeNodePage
	}
	if f.Limit > MaxScopeNodePage {
		f.Limit = MaxScopeNodePage
	}
	return f
}

// After returns the filter positioned to resume after the given record.
func (f ScopeNodeFilter) After(r ScopeNodeRecord) ScopeNodeFilter {
	f.AfterName, f.AfterID = r.Name, r.ID
	return f
}

// PageToken is the opaque cursor a client passes back to resume after this
// record. It carries BOTH name and id because the ordering is (name, id) and
// name is not unique — a token of name alone would skip every sibling sharing
// a name with the last row of a page.
func (r ScopeNodeRecord) PageToken() string {
	return base64.RawURLEncoding.EncodeToString([]byte(r.Name + "\x00" + r.ID))
}

// ErrBadPageToken means the cursor did not come from PageToken.
var ErrBadPageToken = errors.New("malformed page token")

// ParsePageToken decodes a cursor. An empty token means "start at the
// beginning"; a malformed one is an ERROR rather than a silent restart,
// because silently returning page 1 for a corrupted cursor makes a paging
// client loop over the first page forever without ever failing.
func ParsePageToken(tok string) (name, id string, err error) {
	if tok == "" {
		return "", "", nil
	}
	raw, derr := base64.RawURLEncoding.DecodeString(tok)
	if derr != nil {
		return "", "", ErrBadPageToken
	}
	// Split at the LAST separator, not the first: the id is the final field,
	// so a name that itself contains the separator cannot shift the cursor
	// onto a different node.
	decoded := string(raw)
	i := strings.LastIndex(decoded, "\x00")
	if i < 0 || i == len(decoded)-1 {
		return "", "", ErrBadPageToken
	}
	return decoded[:i], decoded[i+1:], nil
}
