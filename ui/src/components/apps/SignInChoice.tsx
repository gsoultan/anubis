import { UnstyledButton } from '@mantine/core'
import type { AppRecord } from '@/lib/api/live'

export type SignIn = AppRecord['sign_in']

/* Who may sign in at all, before any permission is asked about. An app for
   the finance team should not hand a session to everyone in the company who
   happens to have an account. Two cards rather than a toggle: each choice
   needs its sentence, and a phone has no room for both labels on one line. */
export function SignInChoice({ value, onChange, appName }: { value: SignIn; onChange: (v: SignIn) => void; appName?: string }) {
  const name = appName || 'it'
  const choices: { v: SignIn; label: string; hint: string }[] = [
    { v: 'role_holders', label: `Only people given a role in ${name}`,
      hint: 'Everyone else is stopped at sign-in with a clear message, and so is a refresh once their last role ends. Recommended.' },
    { v: 'anyone', label: 'Anyone with an account',
      hint: 'They can sign in. What they see inside still depends on their roles.' },
  ]
  return (
    <div>
      <div className="t-body mb-1.5" style={{ fontWeight: 500 }}>Who may sign in{appName ? ` to ${appName}` : ''}</div>
      <div className="grid gap-2 sm:grid-cols-2" role="radiogroup">
        {choices.map((c) => (
          <UnstyledButton key={c.v} role="radio" aria-checked={value === c.v} onClick={() => onChange(c.v)}
            className="panel-inset px-3 py-2.5"
            style={value === c.v ? { outline: '2px solid var(--accent)', outlineOffset: -2 } : undefined}>
            <div className="t-body" style={{ fontWeight: 550 }}>{c.label}</div>
            <div className="t-xs">{c.hint}</div>
          </UnstyledButton>
        ))}
      </div>
    </div>
  )
}
