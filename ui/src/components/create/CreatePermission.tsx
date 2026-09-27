import { useNavigate } from '@tanstack/react-router'
import { useCreate } from '@/stores/create'
import { CreateShell, CancelSubmit } from './shell'

/* Permissions are not created here, and never were. There is no
   CreatePermission RPC: an application declares the permissions it enforces
   in its manifest, so the catalog and the code that checks it cannot drift.

   This drawer used to be a form that suggested otherwise. It asked for an
   application, a resource and an action, previewed the key — and its button
   could only ever answer "declared by the manifest". Until v0.4.1 it could not
   even do that, because the button never enabled; and it refused names over
   31 characters, a limit the manifest does not have. Every way in — the Add
   menu, the palette, the Roles page, ?new=permission — now lands on the
   answer and a way to act on it, instead of a form. */
// One field per line so it reads at phone width without scrolling sideways.
// internal/authz/app/catalog parses and validates it, as it does the guide's.
const EXAMPLE = `{
  "permissions": [
    {
      "resource": "invoice",
      "action": "approve",
      "description": "Approve an invoice",
      "risk": "sensitive",
      "min_assurance": 2,
      "requires_amr": ["otp"],
      "max_auth_age": "5m"
    }
  ]
}`

export function CreatePermission({ opened }: { opened: boolean }) {
  const { close } = useCreate()
  const navigate = useNavigate()

  return (
    <CreateShell
      opened={opened} onClose={close} title="Add a permission"
      description={<>Permissions are declared by the application that enforces them, in its
        <b> manifest</b> — so the catalog and the code that checks it cannot drift. There is
        nothing to fill in here.</>}
      footer={
        <CancelSubmit onCancel={close} canSubmit submitting={false} label="Open Applications"
          onSubmit={() => { close(); void navigate({ to: '/applications' }) }} />
      }
    >
      <div className="flex flex-col gap-3">
        <p className="t-sm">
          On <b>Applications</b>, choose <b>Manifest</b> on the application that owns it and add
          it under <code>permissions</code>. Only <code>resource</code> and <code>action</code> are
          required; risk, assurance and step-up are optional.
        </p>
        <pre className="panel-inset overflow-x-auto px-3 py-2.5 font-mono"
          style={{ fontSize: 11.5, lineHeight: 1.55 }}>{EXAMPLE}</pre>
        <p className="t-sm">
          It becomes <code>&lt;app&gt;:invoice:approve</code>. <b>Check</b> first — the report says
          what would change before anything does. A spreadsheet works too: a CSV whose header
          starts <code>resource, action</code>.
        </p>
      </div>
    </CreateShell>
  )
}
