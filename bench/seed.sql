-- Realistic volume seed. Two tenants (the second proves isolation).
-- Disable ONLY the catalog-bump triggers. session_replication_role='replica'
-- would also disable data-integrity triggers, which is how the permissions
-- key column silently came out NULL on the first run.

INSERT INTO tenants (slug, name) VALUES
  ('impack','PT Impack Pratama'), ('other','Other Tenant Co');

-- Three identity populations inside ONE tenant. Partners do NOT get their own
-- tenant: that would require cross-tenant grants, and every scope FK is
-- composite on tenant_id precisely to make those impossible.
INSERT INTO realms (tenant_id, code, kind, display_name, min_assurance,
                    self_registration, allowed_factors, required_factors,
                    default_retention, session_ttl)
SELECT t.id, r.code, r.kind, r.dn, r.ia, r.selfreg, r.af, r.rf, r.ret, r.ttl
  FROM tenants t, (VALUES
    ('internal','internal','Internal',             3::smallint, false,
     '{password,totp,device_key}'::text[], '{password,totp}'::text[], NULL::interval, '12 hours'::interval),
    ('partner','partner','Partners', 2::smallint, false,
     '{password,totp}'::text[], '{password,totp}'::text[], NULL::interval, '8 hours'::interval),
    ('public','public','Public',    1::smallint, true,
     '{password,email_otp}'::text[], '{password}'::text[], '2 years'::interval, '1 hour'::interval)
  ) AS r(code,kind,dn,ia,selfreg,af,rf,ret,ttl)
 WHERE t.slug='impack';

-- Categories: runtime data, seeded with the obvious defaults. 'Public' can be
-- anything, which is exactly why these are rows a tenant extends later.
INSERT INTO realm_categories (tenant_id, realm_id, code, display_name, sort_order)
SELECT r.tenant_id, r.id, v.code, v.dn, v.so
  FROM realms r JOIN (VALUES
    ('internal','employee','Employee',10),
    ('partner','supplier','Supplier',10), ('partner','contractor','Contractor',20),
    ('public','applicant','Applicant',10), ('public','customer','Customer',20)
  ) v(realm,code,dn,so) ON r.code = v.realm;

INSERT INTO scope_axes (code, display_name, default_effect, sort_order, resolution) VALUES
  ('org','Organisation','unconstrained',10,'{"from":"token"}'),
  ('partner','Partner Organisation','unconstrained',15,'{"from":"token"}'),
  ('product','Product Line','unconstrained',20,'{"from":"context","key":"product_id"}'),
  ('customer','Customer','unconstrained',30,'{"from":"context","key":"customer_id"}');

INSERT INTO scope_node_types (code, axis_code, display_name, parent_types) VALUES
  ('org','org','Organisation','{}'),
  ('partner_root','partner','All Partners','{}'),
  ('partner_org','partner','Partner Company','{partner_root}'),
  ('office','org','Work Office','{org}'),
  ('division','org','Division','{office}'),
  ('department','org','Department','{office,division}'),
  ('team','org','Team','{department}'),
  ('catalog','product','Catalog','{}'),
  ('product_line','product','Product Line','{catalog}'),
  ('product_family','product','Family','{product_line}'),
  ('sku','product','SKU','{product_family}'),
  ('accounts','customer','Accounts','{}'),
  ('segment','customer','Segment','{accounts}'),
  ('industry','customer','Industry','{segment}'),
  ('account','customer','Account','{industry}');

-- Roots
SELECT scope_ensure_root(t.id, a.code)
  FROM tenants t CROSS JOIN scope_axes a;

-- ── ORG AXIS: 20 offices x 10 departments x 5 teams ────────────────────
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT t.id,'org','office', r.id, 'office-'||g, 'Office '||g
  FROM tenants t
  JOIN scope_nodes r ON r.tenant_id=t.id AND r.axis_code='org' AND r.is_axis_root
 CROSS JOIN generate_series(1,20) g WHERE t.slug='impack';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='office' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='office' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'org','department', p.id, 'dept-'||g, 'Department '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,10) g WHERE p.node_type='office';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='department' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='department' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'org','team', p.id, 'team-'||g, 'Team '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,5) g WHERE p.node_type='department';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='team' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='team' AND NOT n.is_axis_root;

-- ── PRODUCT AXIS: 50 lines x 20 families x 10 SKUs = 10,000 SKUs ───────
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT t.id,'product','product_line', r.id, 'line-'||g, 'Line '||g
  FROM tenants t JOIN scope_nodes r ON r.tenant_id=t.id AND r.axis_code='product' AND r.is_axis_root
 CROSS JOIN generate_series(1,50) g WHERE t.slug='impack';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='product_line' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='product_line' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'product','product_family', p.id, 'fam-'||g, 'Family '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,20) g WHERE p.node_type='product_line';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='product_family' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='product_family' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'product','sku', p.id, 'sku-'||g, 'SKU '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,10) g WHERE p.node_type='product_family';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='sku' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='sku' AND NOT n.is_axis_root;

-- ── CUSTOMER AXIS: 5 segments x 20 industries x 200 accounts = 20,000 ──
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT t.id,'customer','segment', r.id, 'seg-'||g, 'Segment '||g
  FROM tenants t JOIN scope_nodes r ON r.tenant_id=t.id AND r.axis_code='customer' AND r.is_axis_root
 CROSS JOIN generate_series(1,5) g WHERE t.slug='impack';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='segment' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='segment' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'customer','industry', p.id, 'ind-'||g, 'Industry '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,20) g WHERE p.node_type='segment';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='industry' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='industry' AND NOT n.is_axis_root;

INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name)
SELECT p.tenant_id,'customer','account', p.id, 'acct-'||g, 'Account '||g
  FROM scope_nodes p CROSS JOIN generate_series(1,200) g WHERE p.node_type='industry';
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='account' AND NOT n.is_axis_root
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='account' AND NOT n.is_axis_root;

-- ── Applications, permissions, roles ───────────────────────────────────
INSERT INTO applications (tenant_id, slug, name, kind)
SELECT t.id, 'app-'||g, 'App '||g, 'server'
  FROM tenants t CROSS JOIN generate_series(1,10) g WHERE t.slug='impack';

INSERT INTO permissions (application_id, tenant_id, app_slug, resource, action, description)
SELECT a.id, a.tenant_id, a.slug, res, act, ''
  FROM applications a
 CROSS JOIN unnest(ARRAY['invoice','order','employee','report','payment']) res
 CROSS JOIN unnest(ARRAY['read','create','update','approve']) act;

INSERT INTO roles (tenant_id, name, description)
SELECT t.id, 'role-'||g, 'Role '||g
  FROM tenants t CROSS JOIN generate_series(1,50) g WHERE t.slug='impack';

-- Random picks use array indexing off a hashtext(), not ORDER BY md5() in a
-- LATERAL. The latter re-sorts the candidate set once per outer row -- O(n*m)
-- and 30M hash computations at this volume.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
  FROM roles r
  JOIN LATERAL (SELECT id, row_number() OVER (ORDER BY id) rn
                  FROM permissions WHERE tenant_id = r.tenant_id) p
    ON p.rn % 10 = (hashtext(r.id::text) & 2147483647) % 10;

INSERT INTO role_permissions_effective (role_id, permission_id, via_role_id)
SELECT role_id, permission_id, role_id FROM role_permissions;

-- 50,000 identities
INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email)
SELECT t.id, r.id, 3, 'user'||g, 'user'||g||'@impack.test'
  FROM tenants t JOIN realms r ON r.tenant_id=t.id AND r.code='internal'
 CROSS JOIN generate_series(1,50000) g WHERE t.slug='impack';

-- 20 supplier companies on the partner axis
INSERT INTO scope_nodes (tenant_id, axis_code, node_type, parent_id, slug, name, external_ref)
SELECT r.tenant_id,'partner','partner_org', r.id, 'sup-'||g, 'Supplier '||g, 'ERP-SUP-'||g
  FROM scope_nodes r JOIN tenants t ON t.id = r.tenant_id AND t.slug='impack'
 CROSS JOIN generate_series(1,20) g
 WHERE r.axis_code='partner' AND r.is_axis_root;
INSERT INTO scope_closure SELECT c.ancestor_id,n.id,c.depth+1 FROM scope_nodes n
  JOIN scope_closure c ON c.descendant_id=n.parent_id WHERE n.node_type='partner_org'
UNION ALL SELECT n.id,n.id,0 FROM scope_nodes n WHERE n.node_type='partner_org';

-- 2,000 supplier users (assurance 2), 5,000 applicants (assurance 1).
-- Note 'user1' exists in ALL THREE realms: uniqueness is per realm.
INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email, retention_until)
SELECT t.id, r.id, 2, 'user'||g, 'contact'||g||'@supplier.test', NULL
  FROM tenants t JOIN realms r ON r.tenant_id=t.id AND r.code='partner'
 CROSS JOIN generate_series(1,2000) g WHERE t.slug='impack';
INSERT INTO identities (tenant_id, realm_id, assurance_level, username, email, retention_until)
SELECT t.id, r.id, 1, 'user'||g, 'applicant'||g||'@gmail.test', now()+interval '2 years'
  FROM tenants t JOIN realms r ON r.tenant_id=t.id AND r.code='public'
 CROSS JOIN generate_series(1,5000) g WHERE t.slug='impack';

-- assign categories: deterministic split so counts are stable
UPDATE identities i SET category_id = c.id
  FROM realm_categories c JOIN realms r ON r.id = c.realm_id
 WHERE i.realm_id = c.realm_id AND r.code IN ('internal','partner','public')
   AND c.code = CASE
     WHEN r.code='internal' THEN 'employee'
     WHEN r.code='partner' THEN CASE WHEN (hashtext(i.id::text) & 1)=0 THEN 'supplier' ELSE 'contractor' END
     ELSE CASE WHEN (hashtext(i.id::text) & 1)=0 THEN 'applicant' ELSE 'customer' END END;

-- ~150,000 grants: 3 per EMPLOYEE, deterministic role pick.
-- Restricted to the employee realm because the generic seed roles default to
-- allowed_realm_kinds='{employee}' -- granting them to a partner or applicant
-- is exactly what the 0010 guard exists to reject. Partner and applicant grants
-- are created in bench/realms.sql against their own roles.
INSERT INTO grants (tenant_id, identity_id, role_id, granted_by)
SELECT i.tenant_id, i.id,
       ra.a[1 + ((hashtext(i.id::text || k::text) & 2147483647) % array_length(ra.a,1))],
       i.id
  FROM identities i
  JOIN realms rl ON rl.id = i.realm_id AND rl.kind = 'internal'
 CROSS JOIN generate_series(1,3) k
 CROSS JOIN (SELECT array_agg(id) a FROM roles) ra;

-- org constraint on every grant
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'org',
       d.a[1 + ((hashtext(g.id::text) & 2147483647) % array_length(d.a,1))]
  FROM grants g
 CROSS JOIN (SELECT array_agg(id) a FROM scope_nodes WHERE node_type='department') d;

-- product constraint on ~50%
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'product',
       d.a[1 + ((hashtext(g.id::text) & 2147483647) % array_length(d.a,1))]
  FROM grants g
 CROSS JOIN (SELECT array_agg(id) a FROM scope_nodes WHERE node_type='product_line') d
 WHERE (hashtext(g.id::text) & 2147483647) % 2 = 0;

-- customer constraint on ~30%
INSERT INTO grant_scopes (grant_id, tenant_id, axis_code, scope_node_id)
SELECT g.id, g.tenant_id, 'customer',
       d.a[1 + ((hashtext(g.id::text) & 2147483647) % array_length(d.a,1))]
  FROM grants g
 CROSS JOIN (SELECT array_agg(id) a FROM scope_nodes WHERE node_type='industry') d
 WHERE (hashtext(g.id::text) & 2147483647) % 10 < 3;

ANALYZE;
