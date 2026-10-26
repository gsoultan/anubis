package pgsource

// The Osiris tables, read as they are. Osiris is PostgreSQL; its schema is
// what the dump of its database declares, and these statements name only
// columns that exist on every Osiris since the iam/auth split.

const qRules = `SELECT id::text, name, coalesce(description, '') FROM iam.rules ORDER BY name`

const qPolicies = `SELECT id::text, name, coalesce(description, ''), feature_id::text FROM iam.policies ORDER BY name`

const qAuthorities = `SELECT policy_id::text, rule_id::text FROM iam.authorities`

const qFeatures = `SELECT id::text, coalesce(name, ''), module_id::text FROM setting.features`

const qModules = `SELECT id::text, name, app_id::text FROM setting.modules`

const qApps = `SELECT id::text, coalesce(name, '') FROM setting.apps`

const qGroups = `SELECT id::text, code, name, coalesce(description, '') FROM iam.groups ORDER BY code`

// Only GROUP permissions exist in practice; USER is modelled and never written.
const qGroupPolicies = `SELECT scope_id::text, policy_id::text FROM iam.permissions WHERE scope = 'GROUP'`

const qUsers = `SELECT id::text, user_name, entity_type, entity_id::text FROM iam.users`

const qPlacements = `SELECT id::text, entity_id::text,
       coalesce(organization_id::text, ''), coalesce(unit_id::text, ''),
       coalesce(work_place_id::text, ''), coalesce(grade_id::text, ''), coalesce(job_type_id::text, '')
FROM iam.scopes WHERE entity = 'USER'`

const qMembers = `SELECT scope_id::text, group_id::text FROM iam.members`
