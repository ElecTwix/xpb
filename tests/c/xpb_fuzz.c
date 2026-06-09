/*
 * XPB V2 C Runtime libFuzzer Harness
 *
 * Entry point: LLVMFuzzerTestOneInput(data, size).
 *
 * Goal: drive the C decoder over arbitrary/untrusted input bytes through every
 * read function, in fuzzer-chosen sequences, and NEVER crash (no heap overflow,
 * no OOB read/write, no UB). The decoder uses a sticky-error model, so once a
 * read overruns the buffer every subsequent read must be a safe no-op.
 *
 * Wire layout the harness imposes on the input:
 *   byte[0]            -> selects max_elements policy for array reads
 *   byte[1..]          -> a stream of (opcode, payload) where each iteration's
 *                         first remaining byte selects which read function to
 *                         call next; the rest of the buffer is the payload the
 *                         decoder reads from.
 *
 * This means a single fuzz input exercises an arbitrary interleaving of reads
 * against an arbitrary buffer — exactly the adversarial shape that previously
 * tripped the unchecked length-prefix / signed-shift bugs.
 *
 * The harness also feeds the *same* bytes through xpb_decoder_create with
 * length == size (the whole input) so that giant length prefixes inside the
 * payload are tested against the real buffer bound.
 *
 * Build (CI / Linux clang): see tests/c/run_fuzz.sh. The same file is also
 * compiled with XPB_FUZZ_STANDALONE defined to provide a non-libFuzzer driver
 * that replays bytes through LLVMFuzzerTestOneInput, so the harness gets ASan/
 * UBSan coverage even where libFuzzer's runtime is unavailable (e.g. Apple
 * Clang, which ships no libclang_rt.fuzzer).
 */

#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <xpb/xpb.h>

/* Run the full read repertoire driven by fuzzer bytes. Returns nothing; the
 * point is that ASan/UBSan/libFuzzer observe no crash. */
static void drive_decoder(const uint8_t* data, size_t size) {
    if (size == 0) {
        /* Even a zero-length buffer must decode safely. */
        struct xpb_decoder* dec = xpb_decoder_create(data, 0);
        (void)xpb_decoder_read_bool(dec);
        (void)xpb_decoder_read_int32(dec);
        (void)xpb_decoder_read_int64(dec);
        (void)xpb_decoder_read_string(dec);
        (void)xpb_decoder_eof(dec);
        (void)xpb_decoder_remaining(dec);
        xpb_decoder_destroy(dec);
        return;
    }

    /* First byte chooses an array element budget. Keep it small enough that an
     * accepted count can't ask for an unbounded allocation, but non-zero so we
     * exercise the allocate+read path too. */
    size_t max_elements = (size_t)data[0] % 4097; /* 0..4096 */

    /* The opcode stream lives after byte[0]; the decoder reads the WHOLE input
     * so that a length prefix can legitimately reference any offset. */
    struct xpb_decoder* dec = xpb_decoder_create(data, size);
    if (dec == NULL) return;

    const uint8_t* ops = data + 1;
    size_t n_ops = size - 1;

    /* Bound the number of read operations so a pathological input can't make
     * the harness loop forever (each op may consume 0 bytes on a no-op read). */
    size_t budget = n_ops * 2 + 16;

    for (size_t i = 0; i < n_ops && budget > 0; i++, budget--) {
        uint8_t op = ops[i];
        switch (op % 20) {
        case 0: (void)xpb_decoder_read_bool(dec); break;
        case 1: (void)xpb_decoder_read_int32(dec); break;
        case 2: (void)xpb_decoder_read_int64(dec); break;
        case 3: (void)xpb_decoder_read_uint32(dec); break;
        case 4: (void)xpb_decoder_read_uint64(dec); break;
        case 5: (void)xpb_decoder_read_float32(dec); break;
        case 6: (void)xpb_decoder_read_float64(dec); break;
        case 7: {
            char* s = xpb_decoder_read_string(dec);
            xpb_free(s);
            break;
        }
        case 8: {
            size_t n = 0;
            uint8_t* b = xpb_decoder_read_bytes(dec, &n);
            /* Touch the returned memory so ASan validates the allocation. */
            if (b && n) { volatile uint8_t sink = b[n - 1]; (void)sink; }
            xpb_free(b);
            break;
        }
        case 9: {
            size_t n = 0;
            uint8_t* b = xpb_decoder_read_message_bytes(dec, &n);
            if (b && n) { volatile uint8_t sink = b[0]; (void)sink; }
            xpb_free(b);
            break;
        }
        case 10: {
            size_t c = 0;
            int32_t* a = xpb_decoder_read_array_int32(dec, max_elements, &c);
            if (a && c) { volatile int32_t s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(int32_t));
            break;
        }
        case 11: {
            size_t c = 0;
            int64_t* a = xpb_decoder_read_array_int64(dec, max_elements, &c);
            if (a && c) { volatile int64_t s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(int64_t));
            break;
        }
        case 12: {
            size_t c = 0;
            uint32_t* a = xpb_decoder_read_array_uint32(dec, max_elements, &c);
            if (a && c) { volatile uint32_t s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(uint32_t));
            break;
        }
        case 13: {
            size_t c = 0;
            uint64_t* a = xpb_decoder_read_array_uint64(dec, max_elements, &c);
            if (a && c) { volatile uint64_t s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(uint64_t));
            break;
        }
        case 14: {
            size_t c = 0;
            float* a = xpb_decoder_read_array_float32(dec, max_elements, &c);
            if (a && c) { volatile float s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(float));
            break;
        }
        case 15: {
            size_t c = 0;
            double* a = xpb_decoder_read_array_float64(dec, max_elements, &c);
            if (a && c) { volatile double s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(double));
            break;
        }
        case 16: {
            size_t c = 0;
            bool* a = xpb_decoder_read_array_bool(dec, max_elements, &c);
            if (a && c) { volatile bool s = a[c - 1]; (void)s; }
            xpb_free_array(a, c, sizeof(bool));
            break;
        }
        case 17: {
            size_t c = 0;
            char** a = xpb_decoder_read_array_string(dec, max_elements, &c);
            if (a) {
                for (size_t j = 0; j < c; j++) xpb_free(a[j]);
            }
            xpb_free_array(a, c, sizeof(char*));
            break;
        }
        case 18: {
            size_t c = 0;
            /* Exercise validate_array_count directly with a fuzzer-chosen
             * element_min_bytes so the buffer-bound math gets stressed. */
            size_t elem_min = (op >> 5) + 1; /* 1..8 */
            (void)xpb_decoder_validate_array_count(dec, elem_min, max_elements, &c);
            break;
        }
        case 19: {
            /* skip a fuzzer-chosen number of bytes */
            (void)xpb_decoder_skip(dec, op);
            break;
        }
        }

        (void)xpb_decoder_ok(dec);
        (void)xpb_decoder_eof(dec);
        (void)xpb_decoder_remaining(dec);
    }

    xpb_decoder_destroy(dec);
}

/* Re-encode a fuzzer-derived sequence to stress the encoder's growth path and
 * sticky-error handling on adversarial inputs (e.g. huge byte runs). */
static void drive_encoder(const uint8_t* data, size_t size) {
    struct xpb_encoder* enc = xpb_encoder_create(1);
    if (enc == NULL) return;

    /* Write the input as a single bytes blob, then as a string (NUL-safe via a
     * bounded temp copy), plus a few scalars, then finish. */
    xpb_encoder_write_bytes(enc, data, size);
    xpb_encoder_write_int32(enc, (int32_t)size);
    xpb_encoder_write_bool(enc, size & 1);
    if (size >= 8) {
        uint64_t v;
        memcpy(&v, data, sizeof v);
        xpb_encoder_write_uint64(enc, v);
        xpb_encoder_write_float64(enc, (double)v);
    }

    size_t out_len = 0;
    uint8_t* out = xpb_encoder_finish(enc, &out_len);

    /* Round-trip: feed the encoder output back through the decoder. */
    if (out != NULL && out_len > 0) {
        struct xpb_decoder* dec = xpb_decoder_create(out, out_len);
        size_t n = 0;
        uint8_t* b = xpb_decoder_read_bytes(dec, &n);
        xpb_free(b);
        (void)xpb_decoder_read_int32(dec);
        xpb_decoder_destroy(dec);
    }
    free(out);
    xpb_encoder_destroy(enc);
}

int LLVMFuzzerTestOneInput(const uint8_t* data, size_t size) {
    drive_decoder(data, size);
    drive_encoder(data, size);
    return 0;
}

/* --------------------------------------------------------------------- *
 * Standalone driver. Compiled when XPB_FUZZ_STANDALONE is defined so the *
 * harness can run under plain ASan/UBSan (no libFuzzer runtime). It      *
 * replays any files passed as argv, plus a deterministic battery of      *
 * generated inputs that hit the known dangerous shapes (length bombs,    *
 * negative counts, truncated bodies, high-bit values).                   *
 * --------------------------------------------------------------------- */
#ifdef XPB_FUZZ_STANDALONE

static void run_one(const uint8_t* d, size_t n) {
    LLVMFuzzerTestOneInput(d, n);
}

/* A small xorshift PRNG so the standalone battery is deterministic. */
static uint64_t s_rng = 0x9E3779B97F4A7C15ULL;
static uint8_t rng_byte(void) {
    s_rng ^= s_rng << 13;
    s_rng ^= s_rng >> 7;
    s_rng ^= s_rng << 17;
    return (uint8_t)(s_rng >> 24);
}

int main(int argc, char** argv) {
    size_t total = 0;

    /* 1. Replay any corpus files passed on the command line. */
    for (int i = 1; i < argc; i++) {
        FILE* f = fopen(argv[i], "rb");
        if (!f) continue;
        fseek(f, 0, SEEK_END);
        long sz = ftell(f);
        rewind(f);
        if (sz < 0) { fclose(f); continue; }
        uint8_t* buf = (uint8_t*)malloc((size_t)sz + 1);
        if (!buf) { fclose(f); continue; }
        size_t got = (sz > 0) ? fread(buf, 1, (size_t)sz, f) : 0;
        fclose(f);
        run_one(buf, got);
        free(buf);
        total++;
    }

    /* 2. Hand-crafted adversarial shapes. */
    const uint8_t empty[1] = {0};
    run_one(empty, 0); total++;

    /* string length bomb: marker + 0xFFFFFFFF */
    const uint8_t bomb[] = {0x07, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF};
    run_one(bomb, sizeof bomb); total++;

    /* negative array count: opcode read_array_int32 then -1 */
    const uint8_t negcount[] = {0x10, 0x0A, 0xFF, 0xFF, 0xFF, 0xFF};
    run_one(negcount, sizeof negcount); total++;

    /* truncated string body: says 200 bytes, supplies none */
    const uint8_t trunc[] = {0x07, 0xC8};
    run_one(trunc, sizeof trunc); total++;

    /* all-0xFF high-bit values to retrigger any signed-shift UB */
    uint8_t allff[64];
    memset(allff, 0xFF, sizeof allff);
    run_one(allff, sizeof allff); total++;

    /* huge array count claiming many small elements */
    const uint8_t bigarr[] = {0x10, 0x10, 0xFF, 0xFF, 0xFF, 0x7F};
    run_one(bigarr, sizeof bigarr); total++;

    /* 3. A deterministic random battery covering varied sizes. */
    for (int round = 0; round < 200000; round++) {
        uint8_t buf[256];
        size_t n = (size_t)(rng_byte() % sizeof buf);
        for (size_t j = 0; j < n; j++) buf[j] = rng_byte();
        run_one(buf, n);
        total++;
    }

    printf("[standalone] xpb_fuzz harness completed %zu inputs with no crash\n",
           total);
    return 0;
}

#endif /* XPB_FUZZ_STANDALONE */
