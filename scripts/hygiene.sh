#!/usr/bin/env bash
# Repo hygiene: untrack generated/binary artifacts that .gitignore now covers.
# Run from the repo root. Files stay on disk (--cached); history rewrite for the
# large blobs (16MB PDFs) is a separate, deliberate decision — see notes below.
set -euo pipefail

echo "== untracking generated and binary artifacts =="
git rm -r --cached --ignore-unmatch \
  symm.txt \
  frontend/dist \
  .pnpm-store \
  kraken/public/runs \
  kraken/paper/runs \
  market/runs \
  trader/runs \
  logs \
  bin

echo "== done — review with 'git status', then commit =="
echo "note: blobs remain in history; to actually shrink clones use git-lfs migrate"
echo "      or rewrite history (git filter-repo) once, deliberately."
