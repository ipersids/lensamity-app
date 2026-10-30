#!/usr/bin/env bash
set -euo pipefail

RESPONSE_FILE="$(mktemp)"

HTTP_STATUS="$(
  curl --silent --show-error \
    --output "$RESPONSE_FILE" \
    --write-out '%{http_code}' \
    --request POST "https://api.render.com/v1/services/$RENDER_SERVICE_ID/deploys" \
    --header "Authorization: Bearer $RENDER_API_KEY" \
    --header "Content-Type: application/json" \
    --data "$(jq -nc --arg sha "$COMMIT_SHA" '{commitId: $sha}')"
)"

if [ "$HTTP_STATUS" != "201" ]; then
  echo "Render did not create the deploy (HTTP $HTTP_STATUS)."
  cat "$RESPONSE_FILE"
  exit 1
fi

DEPLOY_ID="$(jq -r '.id // empty' "$RESPONSE_FILE")"

test -n "$DEPLOY_ID" || {
  echo "Render response did not contain a deploy ID."
  cat "$RESPONSE_FILE"
  exit 1
}

echo "deploy_id=$DEPLOY_ID" >> "$GITHUB_OUTPUT"
echo "Render deploy started: $DEPLOY_ID"

for attempt in {1..90}; do
  HTTP_STATUS="$(
  curl --silent --show-error \
    --output "$RESPONSE_FILE" \
    --write-out '%{http_code}' \
    --request GET "https://api.render.com/v1/services/${RENDER_SERVICE_ID}/deploys/${DEPLOY_ID}" \
    --header "accept: application/json" \
    --header "Authorization: Bearer ${RENDER_API_KEY}" \
  )"

  if [ "$HTTP_STATUS" != "200" ]; then
    echo "Render retrieve deploy request failed (HTTP $HTTP_STATUS)."
    cat "$RESPONSE_FILE"
    exit 1
  fi

  STATUS="$(jq -r '.status' "$RESPONSE_FILE")"

  case "${STATUS}" in
    created | queued | build_in_progress | update_in_progress)
      echo "Waiting for deployment. Current attempt: ${attempt}"
      sleep 20
      ;;
    live)
      echo "[ SUCCESS ] Render deployment status: $STATUS for deployment id ${DEPLOY_ID}"
      exit 0
      ;;
    deactivated | build_failed | update_failed | canceled | pre_deploy_in_progress | pre_deploy_failed)
      echo "[ FAIL ] Render deployment status: $STATUS for deployment id ${DEPLOY_ID}"
      exit 1
      ;;
    *)
      echo "[ FAIL ] Unexpected Render deployment status: $STATUS"
      exit 1
      ;;
  esac
done

echo "[ FAIL ] Timed out waiting for Render."
exit 1
