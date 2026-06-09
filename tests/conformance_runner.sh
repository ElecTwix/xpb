#!/usr/bin/env bash
#
# Cross-language conformance runner for the Lua and Java xpb runtimes.
#
# Reads the shared golden vectors in testdata/conformance/ (the Go reference
# encoder's .bin files + vectors.json) and runs the Lua and Java conformance
# harnesses, each of which decodes every vector, verifies the decoded values
# against the manifest, re-encodes, and asserts byte-identity.
#
# Detects missing toolchains (lua/luajit, javac/java) and prints SKIP rather
# than failing, so CI can run this unconditionally. Exits non-zero only if a
# harness that *did* run reported a failure.
#
# Usage: tests/conformance_runner.sh
set -u

# Resolve the repo root from this script's location, independent of cwd.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

DATA_DIR="${REPO_ROOT}/testdata/conformance"
MANIFEST="${DATA_DIR}/vectors.json"

overall_rc=0
ran_any=0

hr() { printf '%s\n' "-------------------------------------------"; }

if [ ! -f "${MANIFEST}" ]; then
  echo "[SKIP] conformance manifest not found: ${MANIFEST}"
  echo "       generate it first:"
  echo "         XPB_GEN=1 go test ./tests/conformance/ -run TestGenerateVectors"
  exit 0
fi

# ---------------------------------------------------------------------------
# Lua
# ---------------------------------------------------------------------------
echo "=== Lua conformance ==="
# Prefer a Lua 5.3+ interpreter. xpb.lua uses string.pack/unpack, integer
# division and bitwise operators, so LuaJIT (5.1 semantics) is NOT compatible.
LUA_BIN=""
for cand in lua lua5.4 lua5.3 lua54 lua53; do
  if command -v "${cand}" >/dev/null 2>&1; then
    LUA_BIN="${cand}"
    break
  fi
done

if [ -z "${LUA_BIN}" ]; then
  echo "[SKIP] no Lua 5.3+ interpreter found (tried: lua lua5.4 lua5.3)."
  echo "       note: LuaJIT is unsupported here (xpb.lua needs Lua 5.3+ features)."
else
  echo "Using interpreter: ${LUA_BIN} ($(${LUA_BIN} -v 2>&1 | head -n1))"
  "${LUA_BIN}" "${REPO_ROOT}/tests/lua/conformance.lua"
  lua_rc=$?
  ran_any=1
  if [ "${lua_rc}" -ne 0 ]; then
    echo "[FAIL] Lua conformance exited ${lua_rc}"
    overall_rc=1
  else
    echo "[OK] Lua conformance passed"
  fi
fi
hr

# ---------------------------------------------------------------------------
# Java
# ---------------------------------------------------------------------------
echo "=== Java conformance ==="
if ! command -v javac >/dev/null 2>&1 || ! command -v java >/dev/null 2>&1; then
  echo "[SKIP] no JDK found (need both javac and java on PATH)."
else
  echo "Using JDK: $(javac -version 2>&1)"
  BUILD_DIR="$(mktemp -d)"
  trap 'rm -rf "${BUILD_DIR}"' EXIT
  if javac -d "${BUILD_DIR}" \
        "${REPO_ROOT}/runtime/java/src/main/java/xpb/Encoder.java" \
        "${REPO_ROOT}/runtime/java/src/main/java/xpb/Decoder.java" \
        "${REPO_ROOT}/tests/java/ConformanceTest.java"; then
    # Pass the absolute data dir so the test works regardless of cwd.
    java -cp "${BUILD_DIR}" -DxpbConformanceDir="${DATA_DIR}" xpb.ConformanceTest
    java_rc=$?
    ran_any=1
    if [ "${java_rc}" -ne 0 ]; then
      echo "[FAIL] Java conformance exited ${java_rc}"
      overall_rc=1
    else
      echo "[OK] Java conformance passed"
    fi
  else
    echo "[FAIL] Java conformance compilation failed"
    overall_rc=1
    ran_any=1
  fi
fi
hr

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
if [ "${ran_any}" -eq 0 ]; then
  echo "No conformance harnesses ran (all toolchains skipped)."
  exit 0
fi

if [ "${overall_rc}" -eq 0 ]; then
  echo "All available conformance harnesses passed."
else
  echo "One or more conformance harnesses FAILED."
fi
exit "${overall_rc}"
