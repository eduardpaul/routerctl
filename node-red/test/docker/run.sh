#!/usr/bin/env bash
# Docker end-to-end test of the Node-RED nodes:
#
#   1. builds the routerctl image (Dockerfile) and runs it with the fake
#      driver in demo mode and the API on (token auth, :9100)
#   2. packs this package (npm pack) and installs the tarball into the
#      official nodered/node-red image (as a user would from npm)
#   3. runs flows.json, then check.js: palette, editor endpoints, events,
#      presence, intruder -> approve, queries, actions, policy refusal
#   4. restarts routerctl and checks that the event stream reconnects
#
# Every Node-RED image in NODE_RED_IMAGES is tested (default: 5.x and 4.x).
# Needs docker and node. Nothing touches a real router.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
PKG="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$PKG/.." && pwd)"
WORK="$HERE/.work"
IMAGES="${NODE_RED_IMAGES:-nodered/node-red:5.0.9 nodered/node-red:4.1.17-22}"
TOKEN="e2e-$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
ID="$$"
NET="routerctl-e2e-$ID"
RC="routerctl-e2e-$ID"

# Node-RED writes its data dir as the container user, which the host user
# may not be able to delete (user namespace remapping): each data dir is
# removed from a container, by its exact path
wipe() {
  [ -d "$WORK" ] || return 0
  for d in "$WORK"/data-*; do
    [ -d "$d" ] || continue
    docker run --rm --user 0 -v "$WORK:/w" --entrypoint rm "${IMAGES%% *}" -rf "/w/$(basename "$d")" >/dev/null 2>&1 || true
  done
  rm -rf "$WORK"
}

cleanup() {
  docker rm -f "$RC" >/dev/null 2>&1 || true
  docker ps -aq --filter "name=nodered-e2e-$ID" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  [ -n "${KEEP_WORK:-}" ] || wipe
}
trap cleanup EXIT

wipe
mkdir -p "$WORK"

echo "== build routerctl image"
docker build -q -t routerctl:e2e "$REPO" >/dev/null

echo "== pack the Node-RED package"
TGZ="$(cd "$PKG" && npm pack --silent --pack-destination "$WORK")"
echo "   $TGZ ($(tar tzf "$WORK/$TGZ" | wc -l) files)"
if tar tzf "$WORK/$TGZ" | grep -q 'package/test/\|node_modules'; then
  echo "the package tarball contains tests or node_modules" >&2
  exit 1
fi

docker network create "$NET" >/dev/null
echo "== start routerctl (fake demo device, API with token)"
docker run -d --name "$RC" --network "$NET" --network-alias routerctl \
  -e ROUTERCTL_HTTP_TOKEN="$TOKEN" \
  -v "$HERE/routerctl.yaml:/etc/routerctl/config.yaml:ro" \
  routerctl:e2e watch -c /etc/routerctl/config.yaml >/dev/null

status=0
for IMAGE in $IMAGES; do
  NAME="nodered-e2e-$ID-$(echo "$IMAGE" | tr -c 'a-zA-Z0-9\n' '-')"
  DATA="$WORK/data-$(echo "$IMAGE" | tr -c 'a-zA-Z0-9\n' '_')"
  echo "== $IMAGE"
  mkdir -p "$DATA"
  cp "$HERE/flows.json" "$HERE/settings.js" "$DATA/"
  printf '{"e2e_server":{"token":"%s"}}\n' "$TOKEN" > "$DATA/flows_cred.json"
  printf '{"name":"node-red-e2e","private":true,"dependencies":{"@eduardpaul/node-red-routerctl":"file:/pkg/%s"}}\n' "$TGZ" > "$DATA/package.json"
  chmod -R a+rwX "$DATA"
  docker run --rm -v "$DATA:/data" -v "$WORK:/pkg:ro" -w /data --entrypoint npm "$IMAGE" \
    install --omit=dev --no-audit --no-fund --loglevel=error >/dev/null
  docker run -d --name "$NAME" --network "$NET" -p 127.0.0.1::1880 -v "$DATA:/data" "$IMAGE" >/dev/null
  PORT="$(docker port "$NAME" 1880/tcp | head -1 | sed 's/.*://')"
  if node "$HERE/check.js" "http://127.0.0.1:$PORT" main; then
    echo "   restarting routerctl..."
    node "$HERE/check.js" "http://127.0.0.1:$PORT" boots > "$WORK/boots.json"
    docker restart "$RC" >/dev/null
    if ! node "$HERE/check.js" "http://127.0.0.1:$PORT" restart "$WORK/boots.json"; then
      status=1
      echo "---- Node-RED log"; docker logs --tail 40 "$NAME" 2>&1
      echo "---- routerctl log"; docker logs --tail 15 "$RC" 2>&1
    fi
  else
    status=1
    echo "---- Node-RED log"; docker logs --tail 60 "$NAME" 2>&1
    echo "---- routerctl log"; docker logs --tail 30 "$RC" 2>&1
  fi
  if docker logs "$NAME" 2>&1 | grep -i 'routerctl' | grep -iq 'error\]'; then
    echo "   Node-RED logged routerctl errors:"; docker logs "$NAME" 2>&1 | grep -i 'routerctl' | grep -i 'error\]' | head -5
  fi
  docker rm -f "$NAME" >/dev/null
done

if [ "$status" -eq 0 ]; then echo "== PASS"; else echo "== FAIL"; fi
exit "$status"
