#!/usr/bin/env bash
# Starts a Home Assistant container already carrying the configuration the live
# suite drives, onboards it, and writes an access token to $RUNNER_TEMP/ha_token.
set -euo pipefail

HA_URL="${HA_URL:-http://localhost:8123}"
HA_PORT="${HA_PORT:-8123}"
CONTAINER="${HA_CONTAINER:-ha}"
CONFIG_DIR="${HA_CONFIG_DIR:-${RUNNER_TEMP:-/tmp}/hacfg}"
IMAGE="${HA_IMAGE:-ghcr.io/home-assistant/home-assistant:stable}"
CLIENT_ID="$HA_URL/"

# The entities the live suite drives. demo supplies the lights and climate; the
# helpers come from configuration.yaml below.
REQUIRED_ENTITIES=(
  light.bed_light
  light.ceiling_lights
  climate.heatpump
  input_boolean.live_probe
  input_boolean.porch_motion
  input_boolean.hold_test
  input_number.live_level
)

give_up() {
  echo "$1" >&2
  docker logs --tail 200 "$CONTAINER" >&2
  exit 1
}

# Home Assistant answers HTTP well before it can serve the API, so the onboarding
# endpoint appearing is the first point at which anything may be posted to it.
wait_for_onboarding() {
  local deadline=$((SECONDS + 300))
  while [ "$SECONDS" -lt "$deadline" ]; do
    if curl -sf --max-time 5 "$HA_URL/api/onboarding" >/dev/null; then
      return 0
    fi
    sleep 2
  done
  give_up "Home Assistant never served the onboarding API"
}

# Readiness is every entity the suite needs being present in an authenticated
# /api/states, which is the precondition the tests actually have. Polling the
# frontend instead reports HA as ready while the entity registry is still loading.
wait_for_entities() {
  local token=$1 deadline=$((SECONDS + 600)) present="" missing=""
  while [ "$SECONDS" -lt "$deadline" ]; do
    present=$(curl -sf --max-time 5 "$HA_URL/api/states" -H "Authorization: Bearer $token" |
      python3 -c 'import json,sys; print("\n".join(e["entity_id"] for e in json.load(sys.stdin)))' || true)
    missing=""
    for entity in "${REQUIRED_ENTITIES[@]}"; do
      printf '%s\n' "$present" | grep -qxF "$entity" || missing="$missing $entity"
    done
    if [ -z "$missing" ]; then
      echo "Home Assistant ready with $(printf '%s\n' "$present" | wc -l) entities"
      return 0
    fi
    sleep 2
  done
  give_up "Home Assistant came up without:$missing"
}

# The configuration is written before the container starts rather than applied
# by restarting one. Restarting seconds after onboarding loses the account it
# just created, because Home Assistant defers writing .storage/auth to disk and
# the shutdown beats the flush.
mkdir -p "$CONFIG_DIR"

# Referenced by configuration.yaml, and Home Assistant only creates them itself
# when it is the one generating a default configuration.
: >"$CONFIG_DIR/automations.yaml"
: >"$CONFIG_DIR/scripts.yaml"
: >"$CONFIG_DIR/scenes.yaml"

# A real location, so sun.sun publishes solar times rather than defaults, and
# the helpers the suite drives. demo supplies the lights, covers and climate.
cat >"$CONFIG_DIR/configuration.yaml" <<'YAML'
default_config:

automation: !include automations.yaml
script: !include scripts.yaml
scene: !include scenes.yaml

homeassistant:
  name: CI
  latitude: 29.4241
  longitude: -98.4936
  elevation: 198
  unit_system: metric
  time_zone: America/Chicago
  country: US

logger:
  default: warning

demo:

input_boolean:
  live_probe:
    name: Live Probe
    initial: off
  porch_motion:
    name: Porch Motion
    initial: off
  hold_test:
    name: Hold Test
    initial: off

input_number:
  live_level:
    name: Live Level
    initial: 5
    min: 0
    max: 100
    step: 1
YAML

docker run -d --name "$CONTAINER" -p "$HA_PORT:8123" \
  -v "$CONFIG_DIR:/config" "$IMAGE" >/dev/null

wait_for_onboarding

user_json=$(curl -sf -X POST "$HA_URL/api/onboarding/users" \
  -H 'Content-Type: application/json' \
  -d "{\"client_id\":\"$CLIENT_ID\",\"name\":\"CI\",\"username\":\"ci\",\"password\":\"ci-live-suite-pw\",\"language\":\"en\"}")
auth_code=$(printf '%s' "$user_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["auth_code"])')

token_json=$(curl -sf -X POST "$HA_URL/auth/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=authorization_code&code=$auth_code&client_id=$CLIENT_ID")
token=$(printf '%s' "$token_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
refresh_token=$(printf '%s' "$token_json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["refresh_token"])')

for step in core_config analytics; do
  curl -sf -X POST "$HA_URL/api/onboarding/$step" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' >/dev/null
done
curl -sf -X POST "$HA_URL/api/onboarding/integration" \
  -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d "{\"client_id\":\"$CLIENT_ID\",\"redirect_uri\":\"$CLIENT_ID\"}" >/dev/null

wait_for_entities "$token"

# An access token lasts 30 minutes from when it is issued, so mint the one the
# suite runs under after the waiting is over rather than before it.
curl -sf -X POST "$HA_URL/auth/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=refresh_token&refresh_token=$refresh_token&client_id=$CLIENT_ID" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"], end="")' \
    >"${RUNNER_TEMP:-/tmp}/ha_token"
