#!/usr/bin/env bash
#
# Build + run the XPB C-runtime safety suite:
#   1. libFuzzer campaign over xpb_fuzz.c           (if libFuzzer is available)
#   2. Standalone ASan/UBSan replay of the fuzz harness (always, as fallback +
#      extra coverage)
#   3. ASan/UBSan build+run of the existing tests     (xpb_test, xpb_security_test)
#   4. ASan/UBSan build+run of the conformance harness (xpb_conformance)
#
# Exit non-zero only on a genuine failure (sanitizer crash, test failure,
# fuzzer-found crash). If clang with libFuzzer is unavailable, the fuzzer step
# prints SKIP and is NOT treated as a failure — the rest still runs so the C
# runtime always gets ASan/UBSan coverage and conformance verification.
#
# Usage: tests/c/run_fuzz.sh [seconds]
#   seconds: libFuzzer campaign duration (default 30)

set -u

# ----------------------------------------------------------------------------
# Locate paths relative to this script so it works from any CWD / in CI.
# ----------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
RUNTIME_C="${REPO_ROOT}/runtime/c/xpb.c"
INCLUDE_DIR="${REPO_ROOT}/runtime/c/include"
TESTDATA_DIR="${REPO_ROOT}/testdata/conformance"
FUZZ_SRC="${SCRIPT_DIR}/xpb_fuzz.c"
CONF_SRC="${SCRIPT_DIR}/xpb_conformance.c"
TEST_SRC="${SCRIPT_DIR}/xpb_test.c"
SEC_SRC="${SCRIPT_DIR}/xpb_security_test.c"

MAX_TIME="${1:-30}"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/xpb_fuzz.XXXXXX")"
trap 'rm -rf "${WORKDIR}"' EXIT

CC="${CC:-clang}"
SAN_FLAGS="-fsanitize=address,undefined -fno-sanitize-recover=undefined"
COMMON_FLAGS="-g -O1 -std=c11 -Wall -Wextra -I${INCLUDE_DIR}"

overall_rc=0

hdr() { printf '\n========================================\n%s\n========================================\n' "$1"; }

if ! command -v "${CC}" >/dev/null 2>&1; then
    echo "SKIP: compiler '${CC}' not found; cannot build C safety suite."
    exit 0
fi

# ----------------------------------------------------------------------------
# 1. libFuzzer campaign (best effort).
# ----------------------------------------------------------------------------
hdr "1/4  libFuzzer campaign"

# Probe: can this toolchain link a -fsanitize=fuzzer binary?
cat > "${WORKDIR}/probe.c" <<'EOF'
#include <stdint.h>
#include <stddef.h>
int LLVMFuzzerTestOneInput(const uint8_t *d, size_t n){ (void)d; (void)n; return 0; }
EOF

FUZZER_AVAILABLE=0
if "${CC}" -g -O1 -fsanitize=fuzzer,address,undefined \
        "${WORKDIR}/probe.c" -o "${WORKDIR}/probe" >"${WORKDIR}/probe.log" 2>&1; then
    FUZZER_AVAILABLE=1
fi

if [ "${FUZZER_AVAILABLE}" -eq 1 ]; then
    echo "libFuzzer available — building fuzz target."
    if "${CC}" ${COMMON_FLAGS} -fsanitize=fuzzer,address,undefined \
            "${RUNTIME_C}" "${FUZZ_SRC}" -o "${WORKDIR}/xpb_fuzz"; then
        CORPUS="${WORKDIR}/corpus"
        mkdir -p "${CORPUS}"
        # Seed the corpus with the golden conformance vectors if present.
        if [ -d "${TESTDATA_DIR}" ]; then
            cp "${TESTDATA_DIR}"/*.bin "${CORPUS}/" 2>/dev/null || true
        fi
        echo "Running campaign: -max_total_time=${MAX_TIME} -rss_limit_mb=2048"
        if "${WORKDIR}/xpb_fuzz" \
                -max_total_time="${MAX_TIME}" \
                -rss_limit_mb=2048 \
                -print_final_stats=1 \
                "${CORPUS}"; then
            echo "RESULT: libFuzzer campaign finished with no crash."
        else
            echo "RESULT: libFuzzer FOUND A CRASH (see crash-* artifact above)."
            overall_rc=1
        fi
    else
        echo "SKIP: fuzz target failed to build despite probe success."
    fi
else
    echo "SKIP: clang libFuzzer runtime unavailable on this toolchain."
    echo "      (Apple Clang ships no libclang_rt.fuzzer; install Homebrew LLVM"
    echo "       or run in CI with a fuzzer-capable clang.)"
    echo "      Probe output:"
    sed 's/^/        /' "${WORKDIR}/probe.log" | head -5
    echo "      Falling back to the standalone ASan/UBSan replay below."
fi

# ----------------------------------------------------------------------------
# 2. Standalone ASan/UBSan replay of the fuzz harness (always runs).
# ----------------------------------------------------------------------------
hdr "2/4  Standalone ASan/UBSan fuzz replay"

if "${CC}" ${COMMON_FLAGS} -DXPB_FUZZ_STANDALONE ${SAN_FLAGS} \
        "${RUNTIME_C}" "${FUZZ_SRC}" -o "${WORKDIR}/xpb_fuzz_standalone"; then
    SEEDS=()
    if [ -d "${TESTDATA_DIR}" ]; then
        for f in "${TESTDATA_DIR}"/*.bin; do
            [ -e "$f" ] && SEEDS+=("$f")
        done
    fi
    if "${WORKDIR}/xpb_fuzz_standalone" "${SEEDS[@]}"; then
        echo "RESULT: standalone replay clean under ASan/UBSan."
    else
        echo "RESULT: standalone replay FAILED (sanitizer error)."
        overall_rc=1
    fi
else
    echo "FAIL: could not build standalone fuzz harness."
    overall_rc=1
fi

# ----------------------------------------------------------------------------
# 3. Existing tests under ASan/UBSan.
# ----------------------------------------------------------------------------
hdr "3/4  Existing tests under ASan/UBSan"

run_existing() {
    local name="$1" src="$2"
    echo "--- ${name} ---"
    if "${CC}" ${COMMON_FLAGS} -lm ${SAN_FLAGS} \
            "${RUNTIME_C}" "${src}" -o "${WORKDIR}/${name}"; then
        if "${WORKDIR}/${name}"; then
            echo "RESULT: ${name} passed under ASan/UBSan."
        else
            echo "RESULT: ${name} FAILED under ASan/UBSan."
            overall_rc=1
        fi
    else
        echo "FAIL: could not build ${name}."
        overall_rc=1
    fi
}

run_existing "xpb_test"          "${TEST_SRC}"
run_existing "xpb_security_test" "${SEC_SRC}"

# ----------------------------------------------------------------------------
# 4. Conformance harness under ASan/UBSan.
# ----------------------------------------------------------------------------
hdr "4/4  Conformance harness under ASan/UBSan"

if "${CC}" ${COMMON_FLAGS} ${SAN_FLAGS} \
        "${RUNTIME_C}" "${CONF_SRC}" -o "${WORKDIR}/xpb_conformance"; then
    if "${WORKDIR}/xpb_conformance" "${TESTDATA_DIR}"; then
        echo "RESULT: conformance harness passed (byte-identical re-encode)."
    else
        echo "RESULT: conformance harness FAILED."
        overall_rc=1
    fi
else
    echo "FAIL: could not build conformance harness."
    overall_rc=1
fi

hdr "SUMMARY"
if [ "${overall_rc}" -eq 0 ]; then
    echo "ALL OK (fuzz/replay clean, tests pass, conformance byte-identical)."
else
    echo "FAILURES detected — see sections above."
fi
exit "${overall_rc}"
