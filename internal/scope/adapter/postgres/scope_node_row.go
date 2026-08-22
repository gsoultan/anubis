package scopepg

import scopedomain "github.com/gsoultan/anubis/internal/scope/domain"

func scopeNodeFromRow(id, axis, nodeType, parentID, slug, name, ref, status string, isRoot bool) scopedomain.ScopeNodeRecord {
	return scopedomain.ScopeNodeRecord{
		ID: id, Axis: axis, NodeType: nodeType, ParentID: parentID,
		Slug: slug, Name: name, ExternalRef: ref, Status: status, IsAxisRoot: isRoot,
	}
}
