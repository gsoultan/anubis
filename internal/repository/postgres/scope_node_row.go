package postgres

import "github.com/gsoultan/anubis/internal/repository"

func scopeNodeFromRow(id, axis, nodeType, parentID, slug, name, ref, status string, isRoot bool) repository.ScopeNodeRecord {
	return repository.ScopeNodeRecord{
		ID: id, Axis: axis, NodeType: nodeType, ParentID: parentID,
		Slug: slug, Name: name, ExternalRef: ref, Status: status, IsAxisRoot: isRoot,
	}
}
