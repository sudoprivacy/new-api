# sudoapi: Native offline SudoRouter releases.
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:?usage: bash scripts/package-sudorouter.sh sudorouter-vVERSION ARCH OUTPUT}"
ARCH="${2:?architecture required}"
OUTPUT="${3:?output directory required}"
[[ "$TAG" =~ ^sudorouter-v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
case "$ARCH" in amd64|arm64) ;; *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; esac
VERSION="${TAG#sudorouter-v}"
IMAGE="docker.io/sudoprivacy/new-api:$VERSION-$ARCH"
REVISION="$(git -C "$ROOT" rev-parse HEAD)"
mkdir -p "$OUTPUT"
OUTPUT="$(cd "$OUTPUT" && pwd)"
STAGING="$(mktemp -d)"
CONTAINER=""
cleanup() {
  if [ -n "$CONTAINER" ]; then docker rm -f "$CONTAINER" >/dev/null 2>&1 || true; fi
  rm -rf "$STAGING"
}
trap cleanup EXIT

docker buildx build --platform "linux/$ARCH" --load --tag "$IMAGE" \
  --label "org.opencontainers.image.revision=$REVISION" \
  --label "org.opencontainers.image.version=$VERSION" --file "$ROOT/Dockerfile" "$ROOT"
[ "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$IMAGE")" = "linux/$ARCH" ] \
  || { echo "Image architecture differs from linux/$ARCH" >&2; exit 1; }
# A disposable SQLite instance verifies native startup, migrations and HTTP routing.
# No published port, persistent mount, or provider credentials are needed.
CONTAINER="$(docker run -d --network none --env SQLITE_PATH=/tmp/sudorouter-smoke.db "$IMAGE")"
READY=0
for attempt in $(seq 1 60); do
  if docker exec "$CONTAINER" wget -qO- http://127.0.0.1:3000/api/status > "$STAGING/status.json" \
    && python3 -c 'import json,sys; sys.exit(json.load(open(sys.argv[1])).get("success") is not True)' "$STAGING/status.json"; then
    READY=1
    break
  fi
  [ "$(docker inspect --format '{{.State.Running}}' "$CONTAINER")" = true ] || break
  sleep 1
done
if [ "$READY" != 1 ]; then
  docker logs "$CONTAINER" >&2
  echo 'Router native HTTP smoke failed' >&2
  exit 1
fi
ARCHIVE="sudorouter-$VERSION-linux-$ARCH.tar.gz"
PROVENANCE="sudorouter-$VERSION-linux-$ARCH.source.json"
docker save "$IMAGE" | gzip -1 > "$STAGING/$ARCHIVE"
gzip -t "$STAGING/$ARCHIVE"
IMAGE_ID="$(docker image inspect --format '{{.Id}}' "$IMAGE")"
python3 - "$STAGING/$PROVENANCE" "$TAG" "$ARCH" "$REVISION" "$IMAGE" "$IMAGE_ID" <<'PY'
import json, sys
path, tag, arch, revision, image, image_id = sys.argv[1:]
with open(path, 'w') as stream:
    json.dump(dict(repository='sudoprivacy/new-api', tag=tag, architecture=arch,
                   sourceCommit=revision, image=image, imageId=image_id), stream, indent=2)
    stream.write('\n')
PY
mv "$STAGING/$ARCHIVE" "$STAGING/$PROVENANCE" "$OUTPUT/"
(
  cd "$OUTPUT"
  shasum -a 256 "sudorouter-$VERSION-linux-"*.tar.gz "sudorouter-$VERSION-linux-"*.source.json > SHA256SUMS
)
echo "Prepared $OUTPUT/$ARCHIVE"
