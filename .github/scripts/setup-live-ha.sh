#!/usr/bin/env bash
# Onboards a fresh Home Assistant container and configures the entities the
# live suite drives. Writes an access token to $RUNNER_TEMP/ha_token.
set -euo pipefail

HA_URL="${HA_URL:-http://localhost:8123}"
CONTAINER="${HA_CONTAINER:-ha}"
CLIENT_ID="$HA_URL/"

auth_code=$(curl -sf -X POST "$HA_URL/api/onboarding/users" \
  -H 'Content-Type: application/json' \
  -d "{\"client_id\":\"$CLIENT_ID\",\"name\":\"CI\",\"username\":\"ci\",\"password\":\"ci-live-suite-pw\",\"language\":\"en\"}" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["auth_code"])')

token=$(curl -sf -X POST "$HA_URL/auth/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "grant_type=authorization_code&code=$auth_code&client_id=$CLIENT_ID" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')

for step in core_config analytics; do
  curl -sf -X POST "$HA_URL/api/onboarding/$step" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' >/dev/null
done
curl -sf -X POST "$HA_URL/api/onboarding/integration" \
  -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d "{\"client_id\":\"$CLIENT_ID\",\"redirect_uri\":\"$CLIENT_ID\"}" >/dev/null

# A real location, so sun.sun publishes solar times rather than defaults, and
# the helpers the suite drives. demo supplies the lights, covers and climate.
docker exec -i "$CONTAINER" sh -c 'cat > /config/configuration.yaml' <<'YAML'
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

docker restart "$CONTAINER" >/dev/null

for _ in $(seq 1 90); do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$HA_URL/" || true)
  if [ "$code" = "200" ]; then
    # Serving the frontend precedes the entity registry being populated.
    sleep 10
    printf '%s' "$token" >"${RUNNER_TEMP:-/tmp}/ha_token"
    exit 0
  fi
  sleep 2
done

echo "Home Assistant did not come back after configuration" >&2
docker logs "$CONTAINER" >&2
exit 1
