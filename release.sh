#!/usr/bin/env bash
# release.sh vX.Y.Z — cut a release of this Go module.
# Blocks unless CI is green on HEAD. The git tag IS the module version.
set -euo pipefail
V="${1:?usage: ./release.sh vX.Y.Z}"
[[ "$V" == v*.*.* ]] || { echo "✗ version must look like v0.1.0"; exit 1; }
REPO="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
sha="$(git rev-parse HEAD)"
echo "→ verifying CI green for $sha on $REPO …"
concl="$(gh run list -R "$REPO" --workflow=CI --limit 30 --json headSha,status,conclusion \
  -q "[.[] | select(.headSha==\"$sha\")][0] | .conclusion // .status // \"none\"")"
[ "$concl" = "success" ] || { echo "✗ CI is '$concl' for HEAD — release blocked (push + wait for green)"; exit 1; }
git tag -a "$V" -m "$V"; git push origin "$V"
gh release create "$V" -R "$REPO" --generate-notes --title "$V"
echo "✓ released $V — go get github.com/$REPO@$V"
