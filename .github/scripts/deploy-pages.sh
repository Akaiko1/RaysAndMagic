#!/usr/bin/env bash
# Deploys an uploaded Pages artifact under a build version of its own.
# actions/deploy-pages always passes the commit, and Pages keeps one site per
# build version: a stable tag on the nightly's commit reported success while
# the old site stayed. Polls like the upstream action: only a final status
# stops early; a temporary status or a failed status request keeps polling.
#
# Env: GITHUB_REPOSITORY, GITHUB_SHA, GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT,
# GITHUB_OUTPUT, ACTIONS_ID_TOKEN_REQUEST_URL/TOKEN, GH_TOKEN, ARTIFACT_ID.
# PAGES_TIMEOUT (seconds, default 600) and PAGES_POLL (seconds, default 5)
# exist for the offline test.
set -uo pipefail

timeout="${PAGES_TIMEOUT:-600}"
poll="${PAGES_POLL:-5}"
max_errors=10

oidc=$(curl -sSf --retry 5 --retry-all-errors \
  -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" "${ACTIONS_ID_TOKEN_REQUEST_URL}" | jq -r .value)
if [ -z "${oidc}" ] || [ "${oidc}" = null ]; then
  echo "Could not get an OIDC token for the Pages deployment"
  exit 1
fi
echo "::add-mask::${oidc}"

version="${GITHUB_SHA}-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
created=""
for attempt in 1 2 3 4 5; do
  if created=$(gh api -X POST "repos/${GITHUB_REPOSITORY}/pages/deployments" \
    -F artifact_id="${ARTIFACT_ID}" -f pages_build_version="${version}" -f oidc_token="${oidc}"); then
    break
  fi
  created=""
  sleep $((attempt * poll))
done
if [ -z "${created}" ]; then
  echo "Could not create Pages deployment ${version}"
  exit 1
fi
id=$(jq -r .id <<<"${created}")
echo "page_url=$(jq -r .page_url <<<"${created}")" >> "${GITHUB_OUTPUT}"

errors=0
deadline=$((SECONDS + timeout))
while [ "${SECONDS}" -lt "${deadline}" ]; do
  if status=$(gh api "repos/${GITHUB_REPOSITORY}/pages/deployments/${id}" --jq .status); then
    errors=0
  else
    status=unknown_status
    errors=$((errors + 1))
  fi
  echo "Pages deployment ${version}: ${status}"
  case "${status}" in
    succeed)
      exit 0 ;;
    deployment_failed | deployment_perms_error | deployment_content_failed | deployment_cancelled | deployment_lost)
      exit 1 ;;
  esac
  if [ "${errors}" -ge "${max_errors}" ]; then
    echo "Pages deployment status unavailable ${errors} times in a row"
    exit 1
  fi
  backoff=$((errors * poll))
  [ "${backoff}" -gt 15 ] && backoff=15
  sleep $((poll + backoff))
done
echo "Pages deployment ${version} did not finish in ${timeout}s"
exit 1
