#!/usr/bin/env bash
# Run inside an isolated checkout on the build host. Credentials stay in the
# operator environment; this script never resolves or prints vault contents.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
suite=${1:-go}
case "$suite" in
  go)
    if [[ ${PINELINK_REQUIRE_DATABASE_TESTS:-false} == true ]]; then
      : "${PINELINK_SERVICE_TEST_DATABASE_URL:?a dedicated development test database is required}"
    fi
    for module in backend cli protocol; do
      (cd "$ROOT/$module"; go test -race -p 2 ./...; go vet ./...)
    done
    ;;
  web)
    (cd "$ROOT/web"; npm ci; npm test; npm run build)
    ;;
  flutter)
    (cd "$ROOT/mobile"; flutter pub get; flutter analyze; flutter test)
    ;;
  integration)
    bash "$ROOT/infra/scripts/integration.sh"
    ;;
  *)
    echo "Usage: bash infra/scripts/check.sh {go|web|flutter|integration}" >&2
    exit 2
    ;;
esac
