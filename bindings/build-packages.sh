#!/usr/bin/env bash
# Build everything that goes to a registry, and nothing that goes anywhere.
#
#     bash bindings/build-packages.sh
#
# Leaves, under dist/ at the repository root:
#
#     dist/bin/<goos>_<goarch>/manul[.exe]   the engine, one per target
#     dist/pypi/                             the sdist and one wheel per target
#     dist/npm/                              manul-browser and one engine package per target
#
# It publishes nothing and needs no credentials. The publish workflow runs this
# same script, so what is uploaded is what this produces — there is no second
# description of how a package is put together.
#
# A POSIX host builds all six targets. A Windows host builds the Windows ones
# only: a wheel or a tarball records the mode bits it finds on disk, Windows has
# no executable bit to record, and a Linux engine that installs as a file nobody
# can run is worse than no package. So the full set comes from CI, and a Windows
# checkout can still rehearse the whole path on the targets it can do honestly.
#
# Environment:
#   TARGETS   space-separated GOOS/GOARCH pairs; default as above
#   PYTHON    the interpreter that has `build` installed; default `python`
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
root=$PWD
python=${PYTHON:-python}

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) posix=0 ;;
  *) posix=1 ;;
esac
if [ -z "${TARGETS:-}" ]; then
  if [ "$posix" = 1 ]; then
    TARGETS='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64'
  else
    TARGETS='windows/amd64 windows/arm64'
    echo "Windows host: building the Windows targets only (see the header of this script)."
  fi
fi

# ── one version, or no packages ──────────────────────────────────────────────
#
# The engine's constant is the source of truth and the bindings carry copies. A
# wheel and the engine inside it naming different versions is the one mistake a
# registry will not let anyone take back.
version=$(sed -n 's/^const version = "\(.*\)"$/\1/p' core/cmd/manul/main.go | tr -d '\r')
py_version=$(sed -n 's/^__version__ = "\(.*\)"$/\1/p' bindings/python/manul/__init__.py | tr -d '\r')
node_version=$(node -p "require('./bindings/node/package.json').version")
[ -n "$version" ] || { echo "error: could not read 'const version' from core/cmd/manul/main.go" >&2; exit 1; }
for pair in "python:$py_version" "node:$node_version"; do
  if [ "${pair#*:}" != "$version" ]; then
    echo "error: engine is $version, the ${pair%%:*} binding is ${pair#*:}" >&2
    exit 1
  fi
done
"$python" -c 'import build' 2>/dev/null || {
  echo "error: $python has no 'build' module — $python -m pip install build" >&2
  exit 1
}
echo "manul $version — targets: $TARGETS"

rm -rf dist/bin dist/pypi dist/npm
mkdir -p dist/bin dist/pypi dist/npm

# ── engine ───────────────────────────────────────────────────────────────────
for target in $TARGETS; do
  goos=${target%/*}; goarch=${target#*/}
  name=manul; [ "$goos" = windows ] && name=manul.exe
  mkdir -p "dist/bin/${goos}_${goarch}"
  # The same flags as the release archives: no build-machine paths, symbols
  # left in (see release.yml for why).
  ( cd core && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -o "$root/dist/bin/${goos}_${goarch}/$name" ./cmd/manul )
done

# ── PyPI ─────────────────────────────────────────────────────────────────────
bin=bindings/python/manul/_bin
# The binary is staged inside the package only while its wheel is built. Left
# behind, it would ride along in the next target's wheel — the build hook
# refuses that — or in a developer's editable install.
trap 'rm -rf "$root/$bin"' EXIT

# Source only: the sdist target has no build hook and carries no binary.
( cd bindings/python && "$python" -m build --sdist --outdir "$root/dist/pypi" )

for target in $TARGETS; do
  goos=${target%/*}; goarch=${target#*/}
  rm -rf "$bin"; mkdir -p "$bin"
  cp "dist/bin/${goos}_${goarch}/"manul* "$bin/"
  ( cd bindings/python && MANUL_TARGET="$target" "$python" -m build --wheel --outdir "$root/dist/pypi" )
done
rm -rf "$bin"

# `py3-none-any` in this project always means a wheel was built without its
# engine. The build hook already refuses that; this is the last cheap place to
# be wrong.
"$python" - "$version" dist/pypi/*.whl <<'PY'
import sys, zipfile
version, wheels = sys.argv[1], sys.argv[2:]
bad = 0
for whl in wheels:
    names = zipfile.ZipFile(whl).namelist()
    engines = [n for n in names if n.startswith("manul/_bin/")]
    if whl.endswith("-any.whl") or len(engines) != 1 or f"-{version}-" not in whl:
        print(f"error: {whl}: engines={engines}", file=sys.stderr)
        bad = 1
sys.exit(bad)
PY

# ── npm ──────────────────────────────────────────────────────────────────────
( cd bindings/node
  [ -d node_modules ] || npm ci --no-audit --no-fund
  npm run build
  node scripts/pack.mjs --binaries "$root/dist/bin" --out "$root/dist/npm" )

echo
echo "Built for $version:"
ls -1 dist/pypi dist/npm | sed 's/^/  /'
