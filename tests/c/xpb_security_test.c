/*
 * XPB V2 C Runtime Security Validation Tests
 *
 * Each TestSecurity_XPBxxx test exercises a specific finding from the
 * security audit. These tests should PASS today (fix verified) and FAIL
 * if the hardening is regressed.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <xpb/xpb.h>

static int tests_passed = 0;
static int tests_failed = 0;

#define ASSERT(name, cond) do { \
    if (cond) { \
        printf("  [PASS] %s\n", name); \
        tests_passed++; \
    } else { \
        printf("  [FAIL] %s\n", name); \
        tests_failed++; \
    } \
} while(0)

/*
 * SecurityFinding: XPB-007
 * Severity: Critical
 * Description: Pre-fix, every xpb_decoder_read_* function dereferenced
 *   dec->data[dec->pos] with no bounds check. A 0-byte buffer (or any
 *   buffer where the next read overruns) would read past the end of the
 *   underlying allocation. xpb_decoder_can_read now bounds-checks every
 *   primitive read; the sticky `error` flag prevents subsequent reads
 *   after the first overflow.
 */
static void test_xpb007_read_past_eof() {
    printf("\n=== XPB-007: bounds check on every primitive read ===\n");

    /* Empty buffer; any read must fail safely. */
    uint8_t empty[1] = {0};
    struct xpb_decoder* dec = xpb_decoder_create(empty, 0);

    bool b = xpb_decoder_read_bool(dec);
    ASSERT("read_bool on empty buffer returns 0", b == false);
    ASSERT("decoder marks error after read_bool overrun", !xpb_decoder_ok(dec));

    /* Subsequent reads must also fail (sticky error). */
    int32_t i = xpb_decoder_read_int32(dec);
    ASSERT("read_int32 returns 0 after sticky error", i == 0);
    ASSERT("decoder still in error state", !xpb_decoder_ok(dec));

    xpb_decoder_destroy(dec);

    /* 3-byte buffer; read_int32 (needs 4) must fail without reading. */
    uint8_t three[3] = {0xFF, 0xFF, 0xFF};
    dec = xpb_decoder_create(three, 3);
    int32_t v = xpb_decoder_read_int32(dec);
    ASSERT("read_int32 on 3-byte buffer returns 0", v == 0);
    ASSERT("decoder marked error", !xpb_decoder_ok(dec));
    xpb_decoder_destroy(dec);
}

/*
 * SecurityFinding: XPB-008
 * Severity: Critical
 * Description: Pre-fix, xpb_decoder_read_string did `malloc(len + 1)` with
 *   `len` taken straight from the wire (up to 4 GB). An attacker payload
 *   of {0xFF, 0xFF, 0xFF, 0xFF, 0xFF} (compact-length marker + 0xFFFFFFFF)
 *   would attempt a 4 GB allocation. The fix bounds the length against
 *   the remaining buffer via xpb_decoder_can_read before any malloc.
 */
static void test_xpb008_string_length_bomb() {
    printf("\n=== XPB-008: string length bombs are bounded ===\n");

    /* Compact-length marker (0xFF) + 4-byte length 0xFFFFFFFF. */
    uint8_t bomb[5] = {0xFF, 0xFF, 0xFF, 0xFF, 0xFF};
    struct xpb_decoder* dec = xpb_decoder_create(bomb, 5);

    char* result = xpb_decoder_read_string(dec);
    ASSERT("read_string with 4GB length returns NULL", result == NULL);
    ASSERT("decoder marked error", !xpb_decoder_ok(dec));

    xpb_decoder_destroy(dec);
}

/*
 * SecurityFinding: XPB-009
 * Severity: Critical
 * Description: Pre-fix, xpb_decoder_read_string copied `len` bytes via
 *   memcpy from &dec->data[dec->pos] with no check that pos+len fit
 *   within the buffer. A short buffer with a fake length prefix would
 *   read past the buffer's allocation.
 */
static void test_xpb009_buffer_over_read() {
    printf("\n=== XPB-009: short string with oversized length prefix ===\n");

    /* 1-byte compact length saying "10 bytes follow" but only 0 bytes
     * actually follow. Pre-fix: memcpy reads 10 bytes past dec->data. */
    uint8_t shorty[1] = {10};
    struct xpb_decoder* dec = xpb_decoder_create(shorty, 1);

    char* result = xpb_decoder_read_string(dec);
    ASSERT("read_string with truncated body returns NULL", result == NULL);
    ASSERT("decoder marked error", !xpb_decoder_ok(dec));

    xpb_decoder_destroy(dec);
}

/*
 * SecurityFinding: XPB-010 (signed→unsigned cast on array count)
 * Severity: High
 * Description: Pre-fix, xpb_decoder_read_array_int32 read a signed int32
 *   count, cast it to size_t (negative becomes SIZE_MAX) and stored that
 *   in *out_count. malloc(count * sizeof) then either failed or wrapped
 *   to a small allocation, with the loop driven off the (negative)
 *   int32_t i not iterating at all. Result: caller sees out_count =
 *   SIZE_MAX with no actual elements. The fix rejects negative counts
 *   before allocating.
 */
static void test_xpb010_negative_array_count() {
    printf("\n=== XPB-010: negative array count rejected ===\n");

    /* int32 count = -1 (0xFFFFFFFF little-endian). */
    uint8_t neg[4] = {0xFF, 0xFF, 0xFF, 0xFF};
    struct xpb_decoder* dec = xpb_decoder_create(neg, 4);

    size_t count = 999;
    int32_t* arr = xpb_decoder_read_array_int32(dec, &count);
    ASSERT("read_array_int32 with negative count returns NULL", arr == NULL);
    ASSERT("out_count zeroed (not SIZE_MAX)", count == 0);
    ASSERT("decoder marked error", !xpb_decoder_ok(dec));

    xpb_decoder_destroy(dec);
}

/* Regression: legitimate array round-trip still works after hardening. */
static void test_array_roundtrip_still_works() {
    printf("\n=== Regression: legitimate arrays still round-trip ===\n");

    struct xpb_encoder* enc = xpb_encoder_create(64);
    int32_t in[] = {1, 2, 3, -4, 0x7FFFFFFF};
    xpb_encoder_write_array_int32(enc, in, 5);

    size_t encoded_len = 0;
    uint8_t* encoded = xpb_encoder_finish(enc, &encoded_len);

    struct xpb_decoder* dec = xpb_decoder_create(encoded, encoded_len);
    size_t out_count = 0;
    int32_t* out = xpb_decoder_read_array_int32(dec, &out_count);

    ASSERT("decoded count matches", out_count == 5);
    ASSERT("decoder still ok after legitimate read", xpb_decoder_ok(dec));
    if (out != NULL) {
        ASSERT("element[0]", out[0] == 1);
        ASSERT("element[3]", out[3] == -4);
        ASSERT("element[4]", out[4] == 0x7FFFFFFF);
        free(out);
    }

    free(encoded);
    xpb_decoder_destroy(dec);
    xpb_encoder_destroy(enc);
}

int main() {
    printf("===========================================\n");
    printf("XPB V2 C Security Validation Tests\n");
    printf("===========================================\n");

    test_xpb007_read_past_eof();
    test_xpb008_string_length_bomb();
    test_xpb009_buffer_over_read();
    test_xpb010_negative_array_count();
    test_array_roundtrip_still_works();

    printf("\n===========================================\n");
    printf("Results: %d passed, %d failed\n", tests_passed, tests_failed);
    printf("===========================================\n");

    return tests_failed > 0 ? 1 : 0;
}
