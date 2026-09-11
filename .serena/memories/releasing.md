# Cutting a release (2026-09-07..11)

`release.yml` fires on `push: tags: ["v*"]` — goreleaser, cosign keyless
signing, syft SBOMs, linux amd64+arm64 as deb/rpm/tar.gz.

`.goreleaser.yaml` sets **`draft: true`**. Publishing stays a deliberate human
step; the tag alone builds and signs but ships nothing.

## Tag only a commit whose CI is green

Not "green on the PR" — green on the **merge commit**. `e026fa37` had two
merged, individually-green PRs and still failed on `dev`, because a flaky test
only fails on a contended runner. A signed release cut from a red build is far
harder to undo than a one-CI-cycle delay. Check:

    gh run list --branch dev --limit 1 --json headSha,status,conclusion

## Verify the signature the way a consumer would

A signed release nobody can verify is theatre:

    cosign verify-blob checksums.txt \
      --signature checksums.txt.sig --certificate checksums.txt.pem \
      --certificate-identity-regexp 'https://github.com/gsoultan/anubis' \
      --certificate-oidc-issuer 'https://token.actions.githubusercontent.com'
    # → Verified OK

## Superseding an unpublished draft

`gh release delete <tag> --yes` drops the draft and **keeps the tag** (only
`--cleanup-tag` removes it). That is how v0.3.0 was retired in favour of
v0.3.1 three days later: the tag stays for an honest history, but no release
appears that nobody should install.

## Version choice

Pre-1.0, minor bumps carry deliberate behaviour changes. v0.3.0 earned one
(container defaults `ANUBIS_ENV=prod`; `GetSigninPage`/`PutSigninPage`
deprecated; `UpdateApplication` refuses a changed `slug`/`kind` instead of
ignoring it). v0.3.1 was a patch that still changed what users *see* — the
sign-out page fix in [[page-resolution]] — so the tag message leads with that
rather than burying it under "bug fixes".

See [[core]].
