#!/usr/bin/env bash
# Offline test of deploy-pages.sh: fake curl and gh replay a status sequence.
# Each case names the statuses the status endpoint returns in turn ("error"
# makes that request fail) and the exit code the script must end with.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
failed=0

run_case() {
  local name="$1" want="$2" create="$3"
  shift 3
  local dir
  dir="$(mktemp -d)"
  printf '%s\n' "$@" > "${dir}/statuses"
  echo 0 > "${dir}/calls"
  cat > "${dir}/curl" <<'SH'
#!/usr/bin/env bash
echo '{"value":"fake-oidc"}'
SH
  cat > "${dir}/gh" <<SH
#!/usr/bin/env bash
if [ "\$1 \$2" = "api -X" ]; then
  if [ "${create}" = fail ]; then exit 1; fi
  echo '{"id":"42","page_url":"https://example.invalid/"}'
  exit 0
fi
n=\$(( \$(cat "${dir}/calls") + 1 )); echo "\$n" > "${dir}/calls"
status=\$(sed -n "\${n}p" "${dir}/statuses")
[ -z "\${status}" ] && status=deployment_in_progress
[ "\${status}" = error ] && exit 1
echo "\${status}"
SH
  chmod +x "${dir}/curl" "${dir}/gh"
  PATH="${dir}:${PATH}" GITHUB_REPOSITORY=o/r GITHUB_SHA=abc GITHUB_RUN_ID=1 GITHUB_RUN_ATTEMPT=1 \
    GITHUB_OUTPUT="${dir}/out" ACTIONS_ID_TOKEN_REQUEST_URL=x ACTIONS_ID_TOKEN_REQUEST_TOKEN=y \
    GH_TOKEN=z ARTIFACT_ID=7 PAGES_POLL=0 PAGES_TIMEOUT=5 \
    bash "${here}/deploy-pages.sh" > "${dir}/log" 2>&1
  local got=$?
  if [ "${got}" -ne "${want}" ]; then
    echo "${name}: exit ${got}, want ${want}"
    sed 's/^/    /' "${dir}/log"
    failed=1
  else
    echo "${name}: passed"
  fi
  rm -rf "${dir}"
}

run_case "success" 0 ok deployment_in_progress succeed
run_case "attempt error is retried" 0 ok deployment_attempt_error succeed
run_case "not found is retried" 0 ok not_found deployment_in_progress succeed
run_case "failed status request is retried" 0 ok error error succeed
run_case "final failure stops" 1 ok deployment_in_progress deployment_failed
run_case "permissions failure stops" 1 ok deployment_perms_error
run_case "status never answers" 1 ok error error error error error error error error error error error
run_case "deployment cannot be created" 1 fail
run_case "never finishes" 1 ok
exit "${failed}"
