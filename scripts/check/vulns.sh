#!/usr/bin/env bash
# Known vulnerabilities in what we actually ship.
#
# The SBOM published with each release makes "are we exposed to CVE-X"
# answerable; this asks the question on every push so nobody has to remember
# to. govulncheck is narrower than a dependency scanner on purpose: it reports
# a vulnerability only when the vulnerable SYMBOL is reachable from this code,
# so it does not cry wolf about a CVE in a function nothing calls.
#
# Both modules are scanned. pkg/anubis is the verifier SDK that consuming
# applications compile into themselves, so a vulnerability there travels
# further than one here.
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="$(go env GOPATH)/bin:$PATH"

if ! command -v govulncheck >/dev/null 2>&1; then
  go install golang.org/x/vuln/cmd/govulncheck@latest
fi

# A newly published advisory will fail this on a push that changed nothing.
# That is the intended behaviour: the exposure is real whether or not the
# commit caused it, and finding out from CI beats finding out from a report.
govulncheck ./... >/dev/null
( cd pkg/anubis && govulncheck ./... >/dev/null )

echo "ok: no known vulnerabilities reachable from either module"
