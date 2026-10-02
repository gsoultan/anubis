import { ConnectError } from '@connectrpc/connect'
import { ErrorInfoSchema } from '@/gen/anubis/v1/common_pb'

/* What a refused request says to the person who sent it.
 *
 * The toast used to print the transport's own string:
 *   [invalid_argument] invalid_argument: Invalid argument: storm: check constraint violated
 * — which names the layer that noticed and nothing about what to do. The
 * server already sends the useful part beside it, as details: the field that
 * is wrong, or the sentence the database guard raised (`reason`).
 */

/** The sentence, from a rejection's parts. Pure, so ui/test can hold it. */
export function sayRejection(raw: string, details: Record<string, string>): string {
  const reason = details['reason']?.trim()
  if (reason) return sentence(reason)
  const fields = Object.entries(details).filter(([, v]) => v.trim() !== '')
  if (fields.length > 0) return fields.map(([k, v]) => `${k.replace(/_/g, ' ')}: ${v}`).join(' · ')
  // No details: the message, without the machine code in front of it or the
  // wrapped cause behind it.
  const said = raw.replace(/^[a-z_]+:\s*/, '').split(/:\s/)[0]?.trim()
  return sentence(said || raw || 'The request was refused.')
}

function sentence(s: string): string {
  const t = s.charAt(0).toUpperCase() + s.slice(1)
  return /[.!?]$/.test(t) ? t : `${t}.`
}

export function explain(err: unknown): string {
  if (err instanceof ConnectError) {
    const info = err.findDetails(ErrorInfoSchema)[0]
    return sayRejection(err.rawMessage, info?.details ?? {})
  }
  return err instanceof Error ? err.message : String(err)
}
