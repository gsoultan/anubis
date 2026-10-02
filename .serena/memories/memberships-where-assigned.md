# Memberships: places per member, assignments as rows (0053–0054, 2026-10-02)

A membership is a named set of roles. Two kinds, fixed at creation
(`memberships.anchor_axis`, guarded by `memberships_anchor_fixed`):

- **Same places for everyone** (`anchor_axis` NULL): each entry names its
  places; every member gets exactly those. The only kind before 0054.
- **Where each member is assigned** (`anchor_axis` = a structure): the place
  lives on the assignment. A seat on the Marketing Council in Company A gives
  nothing in Company B. An entry may add limits in OTHER structures only.

## An assignment is a row
`membership_members` has its own `id`, a place (`axis_code`+`scope_node_id`),
`inherit`, `valid_until`, `reason`, and `removed_at/removed_by`. One person can
hold one membership at several places. `grants.via_member_id` says which
assignment gave a grant, so leaving one place takes back that place's grants
only. Proto: `Grant.via_assignment_id`, `ListMembershipAssignments`,
`UnassignMembership.assignment_id`.

## Things that were wrong, and why
- **Contents froze after the first member.** Replacing entries DELETEd them;
  grants point back with RESTRICT and are kept after revoke. Entries are now
  retired (`retired_at`), and the replace is a multiset diff so an unchanged
  entry keeps its id and its members' grants.
- **Revoke overwrote the grant's reason** — `grants.revoke_reason` (0053).
- **Membership writes took the id on trust** across tenants. The interactor
  discarded the principal after the guard ([[tenant-scoped-reads]]);
  `membershipInTenant` is the fix, `TestMembershipWritesRefuseAnotherTenantsMembership` the proof.
- **`grants_created` was always 0**: a statement cannot see rows a volatile
  function inserted in that same statement. Assign, then count, in one tx.

## Console
`components/access/MembershipEntries` is the one editor (create + edit).
`routes/memberships.tsx`: mode line, what it gives, a roster that loads only
when opened (keyset pages of 25), remove per assignment, Edit, Add member —
which opens the give-access sheet with the membership preset
([[console-give-access]]). A person's page lists one group per assignment.

## Tests
`test/integration/memberships/` (own package: the parent folder is at the
ten-file limit). Each test builds its own structure and rolls back.
