package scopepg

import scopedomain "github.com/gsoultan/anubis/internal/scope/domain"

func scopeNodeFromRow(id, axis, nodeType, parentID, parentAxis, slug, name, ref, status string, isRoot bool, children int32) scopedomain.ScopeNodeRecord {
	return scopedomain.ScopeNodeRecord{
		ID: id, Axis: axis, NodeType: nodeType,
		ParentID: parentID, ParentAxis: parentAxis,
		Slug: slug, Name: name, ExternalRef: ref, Status: status, IsAxisRoot: isRoot,
		ChildCount: int(children),
	}
}
