#!/bin/sh
set -eu

root=$(git rev-parse --show-toplevel)
cd "$root"

self=tests/security/public-tree-privacy-test.sh
failed=0

report_matches() {
    description=$1
    matches=$2
    if [ -n "$matches" ]; then
        printf '%s\n%s\n' "public-tree privacy check failed: $description" "$matches" >&2
        failed=1
    fi
}

matches=$(git grep -nEI \
    '([[:alnum:]._%+-]+@[[:alnum:].-]+\.[[:alpha:]]{2,})' \
    -- . ":!$self" 2>/dev/null | grep -vE 'git@github\.com' || true)
report_matches 'email address found' "$matches"

matches=$(git grep -nEI \
    '(^|[^0-9])(10\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]{1,3}\.[0-9]{1,3}|192\.168\.[0-9]{1,3}\.[0-9]{1,3}|169\.254\.[0-9]{1,3}\.[0-9]{1,3})([^0-9]|$)' \
    -- . ":!$self" 2>/dev/null |
    grep -vE '10\.77\.0\.(1|20|100)([^0-9]|$)' || true)
report_matches 'private or link-local runtime address found' "$matches"

matches=$(git grep -nEI \
    '/(home|Users|media)/[^[:space:]`"]+' \
    -- . ":!$self" 2>/dev/null || true)
report_matches 'local user or media path found' "$matches"

matches=$(git grep -nEI \
    '/dev/sd[a-z][0-9]*' \
    -- . ":!$self" 2>/dev/null || true)
report_matches 'concrete local block-device path found' "$matches"

matches=$(git grep -nEI \
    '(host_cpu|"cpu"|Host:).*(AMD|Intel|Apple M[0-9])' \
    -- . ":!$self" 2>/dev/null || true)
report_matches 'local host hardware fingerprint found' "$matches"

matches=$(git grep -nEI \
    "(ADMIN_TOKEN|PASSWORD|PASSWD|API_KEY|SECRET)[[:space:]]*[:=][[:space:]]*[\"']?[[:xdigit:]]{40,}" \
    -- . ":!$self" 2>/dev/null || true)
report_matches 'possible hardcoded credential found' "$matches"

if [ "$failed" -ne 0 ]; then
    exit 1
fi

echo 'public-tree privacy check: PASS'
