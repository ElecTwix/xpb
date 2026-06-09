/*
 * XPB V2 C Runtime Cross-Language Conformance Harness
 *
 * Reads the shared golden vectors in testdata/conformance (the *.bin set,
 * produced by
 * the Go reference encoder), decodes them with the C runtime, verifies the
 * decoded values against an in-source expected table, then re-encodes them and
 * asserts the result is byte-identical to the .bin file.
 *
 * Parsing testdata/conformance/vectors.json in C is heavy, so the per-vector
 * expected values/ops are hardcoded here in a static table keyed by vector
 * filename. The .bin files themselves are NEVER hardcoded: they are read from
 * disk at runtime and the re-encode is asserted byte-for-byte against them.
 *
 * Floats are compared by bit pattern so NaN / -0.0 / +/-inf survive exactly.
 *
 * The testdata directory is located via (in priority order):
 *   1. argv[1]                         (explicit path to testdata/conformance)
 *   2. $XPB_CONFORMANCE_DIR            (env override)
 *   3. walking up from the source file dir to find testdata/conformance
 *   4. a few common relative guesses
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <inttypes.h>
#include <sys/stat.h>
#include <xpb/xpb.h>

static int g_pass = 0;
static int g_fail = 0;

#define CHECK(name, cond) do { \
    if (cond) { g_pass++; } \
    else { g_fail++; printf("  [FAIL] %s\n", (name)); } \
} while (0)

/* ----------------------------------------------------------------------- *
 * Op model: a small tagged union mirroring tests/conformance/manifest.go.  *
 * ----------------------------------------------------------------------- */

typedef enum {
    OP_BOOL, OP_INT32, OP_INT64, OP_UINT32, OP_UINT64,
    OP_FLOAT32, OP_FLOAT64, OP_STRING, OP_BYTES,
    OP_ARRAY, OP_MAP, OP_MESSAGE
} op_kind;

typedef enum {
    T_BOOL, T_INT32, T_INT64, T_UINT32, T_UINT64,
    T_FLOAT32, T_FLOAT64, T_STRING, T_BYTES
} scalar_type;

struct op;

typedef struct {
    const struct op* k;
    const struct op* v;
} map_entry;

typedef struct op {
    op_kind kind;

    /* scalars */
    bool        b;
    int32_t     i32;
    uint32_t    u32;
    int64_t     i64;
    uint64_t    u64;
    uint32_t    f32_bits;   /* float32 stored as bit pattern */
    uint64_t    f64_bits;   /* float64 stored as bit pattern */
    const char* str;        /* for OP_STRING (NUL-terminated) */
    const uint8_t* bytes;    /* for OP_BYTES */
    size_t      bytes_len;

    /* array */
    scalar_type      elem_type;
    const struct op* elems;
    size_t           n_elems;

    /* map */
    scalar_type        key_type;
    scalar_type        val_type;
    const map_entry*   entries;
    size_t             n_entries;

    /* message */
    const struct op* ops;
    size_t           n_ops;
} op;

/* Constructors (compound-literal helpers keep the table compact). */
#define OB(x)        ((op){ .kind = OP_BOOL,    .b = (x) })
#define OI32(x)      ((op){ .kind = OP_INT32,   .i32 = (x) })
#define OU32(x)      ((op){ .kind = OP_UINT32,  .u32 = (x) })
#define OI64(x)      ((op){ .kind = OP_INT64,   .i64 = (x) })
#define OU64(x)      ((op){ .kind = OP_UINT64,  .u64 = (x) })
#define OF32(bits)   ((op){ .kind = OP_FLOAT32, .f32_bits = (bits) })
#define OF64(bits)   ((op){ .kind = OP_FLOAT64, .f64_bits = (bits) })
#define OS(x)        ((op){ .kind = OP_STRING,  .str = (x) })
#define OBY(p, n)    ((op){ .kind = OP_BYTES,   .bytes = (p), .bytes_len = (n) })

typedef struct {
    const char* file;   /* .bin filename inside testdata/conformance */
    const op*   ops;
    size_t      n_ops;
} vector;

/* ----------------------------------------------------------------------- *
 * Expected vector definitions (transcribed from vectors.json).            *
 * ----------------------------------------------------------------------- */

/* Build long repeated-byte payloads at startup to avoid giant literals. */
static char  s_a254[255];
static char  s_b255[256];
static char  s_c256[257];
static char  s_z1000[1001];
static uint8_t s_ab255[255];

static void init_long_payloads(void) {
    memset(s_a254, 'a', 254); s_a254[254] = '\0';
    memset(s_b255, 'b', 255); s_b255[255] = '\0';
    memset(s_c256, 'c', 256); s_c256[256] = '\0';
    memset(s_z1000, 'Z', 1000); s_z1000[1000] = '\0';
    memset(s_ab255, 0xab, 255);
}

/* Scalars / simple vectors. */
static const op v_bool_true[]   = { OB(true) };
static const op v_bool_false[]  = { OB(false) };
static const op v_int32_zero[]  = { OI32(0) };
static const op v_int32_neg1[]  = { OI32(-1) };
static const op v_int32_max[]   = { OI32(2147483647) };
static const op v_int32_min[]   = { OI32(-2147483647 - 1) };
static const op v_int32_sample[]= { OI32(30) };
static const op v_int64_zero[]  = { OI64(0) };
static const op v_int64_neg1[]  = { OI64(-1) };
static const op v_int64_max[]   = { OI64(9223372036854775807LL) };
static const op v_int64_min[]   = { OI64(-9223372036854775807LL - 1) };
static const op v_uint32_zero[] = { OU32(0) };
static const op v_uint32_max[]  = { OU32(4294967295U) };
static const op v_uint64_zero[] = { OU64(0) };
static const op v_uint64_max[]  = { OU64(18446744073709551615ULL) };

static const op v_f32_zero[]    = { OF32(0x00000000U) };
static const op v_f32_negzero[] = { OF32(0x80000000U) };
static const op v_f32_pi[]      = { OF32(0x4048F5C3U) };
static const op v_f32_posinf[]  = { OF32(0x7F800000U) };
static const op v_f32_neginf[]  = { OF32(0xFF800000U) };
static const op v_f32_nan[]     = { OF32(0x7FC00000U) };

static const op v_f64_zero[]    = { OF64(0x0000000000000000ULL) };
static const op v_f64_negzero[] = { OF64(0x8000000000000000ULL) };
static const op v_f64_pi[]      = { OF64(0x400921FB54442D18ULL) };
static const op v_f64_posinf[]  = { OF64(0x7FF0000000000000ULL) };
static const op v_f64_neginf[]  = { OF64(0xFFF0000000000000ULL) };
static const op v_f64_nan[]     = { OF64(0x7FF8000000000000ULL) };

static const op v_string_empty[] = { OS("") };
static const op v_string_ascii[] = { OS("Alice") };
/* héllo 世界 🚀  -> UTF-8 escape literal. */
static const op v_string_utf8[]  = { OS("h\xc3\xa9llo \xe4\xb8\x96\xe7\x95\x8c \xf0\x9f\x9a\x80") };
static op v_string_len254[1];
static op v_string_len255[1];
static op v_string_len256[1];
static op v_string_long[1];

static const op v_bytes_empty[]  = { OBY(NULL, 0) };
static const uint8_t bn[] = {0x00,0x01,0xfe,0xff,0x7f,0x80};
static const op v_bytes_nonempty[] = { OBY(bn, sizeof bn) };
static op v_bytes_len255[1];

/* array_int32_empty: array<int32> with no elems. */
static const op v_array_i32_empty[] = {
    { .kind = OP_ARRAY, .elem_type = T_INT32, .elems = NULL, .n_elems = 0 }
};
static const op array_i32_elems[] = { OI32(1), OI32(2), OI32(3) };
static const op v_array_i32[] = {
    { .kind = OP_ARRAY, .elem_type = T_INT32, .elems = array_i32_elems, .n_elems = 3 }
};
static const op array_str_elems[] = { OS("a"), OS("bb"), OS("") };
static const op v_array_str[] = {
    { .kind = OP_ARRAY, .elem_type = T_STRING, .elems = array_str_elems, .n_elems = 3 }
};
static const op array_f64_elems[] = {
    OF64(0x3FF8000000000000ULL), OF64(0xC002000000000000ULL)
};
static const op v_array_f64[] = {
    { .kind = OP_ARRAY, .elem_type = T_FLOAT64, .elems = array_f64_elems, .n_elems = 2 }
};

/* maps */
static const op m_si_k0 = OS("a"), m_si_v0 = OI32(1);
static const op m_si_k1 = OS("b"), m_si_v1 = OI32(2);
static const map_entry map_si_entries[] = {
    { &m_si_k0, &m_si_v0 }, { &m_si_k1, &m_si_v1 }
};
static const op v_map_empty[] = {
    { .kind = OP_MAP, .key_type = T_STRING, .val_type = T_INT32, .entries = NULL, .n_entries = 0 }
};
static const op v_map_si[] = {
    { .kind = OP_MAP, .key_type = T_STRING, .val_type = T_INT32,
      .entries = map_si_entries, .n_entries = 2 }
};
static const op m_is_k0 = OI64(100),  m_is_v0 = OS("hundred");
static const op m_is_k1 = OI64(-7),   m_is_v1 = OS("neg");
static const map_entry map_is_entries[] = {
    { &m_is_k0, &m_is_v0 }, { &m_is_k1, &m_is_v1 }
};
static const op v_map_is[] = {
    { .kind = OP_MAP, .key_type = T_INT64, .val_type = T_STRING,
      .entries = map_is_entries, .n_entries = 2 }
};

/* messages */
static const op msg_simple_ops[] = { OS("Bob"), OI32(25), OB(true) };
static const op v_message_simple[] = {
    { .kind = OP_MESSAGE, .ops = msg_simple_ops, .n_ops = 3 }
};
static const op msg_nested_inner[] = { OS("NYC"), OS("USA") };
static const op v_message_nested[] = {
    OS("Alice"),
    { .kind = OP_MESSAGE, .ops = msg_nested_inner, .n_ops = 2 }
};
static const op v_message_empty[] = {
    { .kind = OP_MESSAGE, .ops = NULL, .n_ops = 0 }
};

/* mixed_record */
static const op mixed_arr_elems[] = { OS("admin"), OS("user") };
static const uint8_t mixed_deadbeef[] = {0xde,0xad,0xbe,0xef};
static const op mixed_msg_ops[] = {
    OI64(9223372036854775807LL),
    OU64(18446744073709551615ULL),
    OBY(mixed_deadbeef, 4),
};
static const op v_mixed_record[] = {
    OS("user-42"),
    OI32(42),
    OB(true),
    OF64(0x4058A66666666666ULL),
    { .kind = OP_ARRAY, .elem_type = T_STRING, .elems = mixed_arr_elems, .n_elems = 2 },
    { .kind = OP_MESSAGE, .ops = mixed_msg_ops, .n_ops = 3 },
};

#define V(name_, arr_) { name_, arr_, sizeof(arr_) / sizeof((arr_)[0]) }

static vector g_vectors[] = {
    V("bool_true.bin",        v_bool_true),
    V("bool_false.bin",       v_bool_false),
    V("int32_zero.bin",       v_int32_zero),
    V("int32_neg1.bin",       v_int32_neg1),
    V("int32_max.bin",        v_int32_max),
    V("int32_min.bin",        v_int32_min),
    V("int32_sample.bin",     v_int32_sample),
    V("int64_zero.bin",       v_int64_zero),
    V("int64_neg1.bin",       v_int64_neg1),
    V("int64_max.bin",        v_int64_max),
    V("int64_min.bin",        v_int64_min),
    V("uint32_zero.bin",      v_uint32_zero),
    V("uint32_max.bin",       v_uint32_max),
    V("uint64_zero.bin",      v_uint64_zero),
    V("uint64_max.bin",       v_uint64_max),
    V("float32_zero.bin",     v_f32_zero),
    V("float32_neg_zero.bin", v_f32_negzero),
    V("float32_pi.bin",       v_f32_pi),
    V("float32_pos_inf.bin",  v_f32_posinf),
    V("float32_neg_inf.bin",  v_f32_neginf),
    V("float32_nan.bin",      v_f32_nan),
    V("float64_zero.bin",     v_f64_zero),
    V("float64_neg_zero.bin", v_f64_negzero),
    V("float64_pi.bin",       v_f64_pi),
    V("float64_pos_inf.bin",  v_f64_posinf),
    V("float64_neg_inf.bin",  v_f64_neginf),
    V("float64_nan.bin",      v_f64_nan),
    V("string_empty.bin",     v_string_empty),
    V("string_ascii.bin",     v_string_ascii),
    V("string_utf8.bin",      v_string_utf8),
    V("string_len254.bin",    v_string_len254),
    V("string_len255.bin",    v_string_len255),
    V("string_len256.bin",    v_string_len256),
    V("string_long.bin",      v_string_long),
    V("bytes_empty.bin",      v_bytes_empty),
    V("bytes_nonempty.bin",   v_bytes_nonempty),
    V("bytes_len255.bin",     v_bytes_len255),
    V("array_int32_empty.bin",v_array_i32_empty),
    V("array_int32.bin",      v_array_i32),
    V("array_string.bin",     v_array_str),
    V("array_float64.bin",    v_array_f64),
    V("map_empty.bin",        v_map_empty),
    V("map_string_int32.bin", v_map_si),
    V("map_int64_string.bin", v_map_is),
    V("message_simple.bin",   v_message_simple),
    V("message_nested.bin",   v_message_nested),
    V("message_empty.bin",    v_message_empty),
    V("mixed_record.bin",     v_mixed_record),
};
#define N_VECTORS (sizeof(g_vectors) / sizeof(g_vectors[0]))

static void init_dynamic_vectors(void) {
    init_long_payloads();
    v_string_len254[0] = OS(s_a254);
    v_string_len255[0] = OS(s_b255);
    v_string_len256[0] = OS(s_c256);
    v_string_long[0]   = OS(s_z1000);
    v_bytes_len255[0]  = OBY(s_ab255, sizeof s_ab255);
}

/* ----------------------------------------------------------------------- *
 * Encode a sequence of ops with the C encoder.                            *
 * ----------------------------------------------------------------------- */

static void encode_op(struct xpb_encoder* enc, const op* o) {
    switch (o->kind) {
    case OP_BOOL:    xpb_encoder_write_bool(enc, o->b); break;
    case OP_INT32:   xpb_encoder_write_int32(enc, o->i32); break;
    case OP_INT64:   xpb_encoder_write_int64(enc, o->i64); break;
    case OP_UINT32:  xpb_encoder_write_uint32(enc, o->u32); break;
    case OP_UINT64:  xpb_encoder_write_uint64(enc, o->u64); break;
    case OP_FLOAT32: {
        float f; memcpy(&f, &o->f32_bits, sizeof f);
        xpb_encoder_write_float32(enc, f);
        break;
    }
    case OP_FLOAT64: {
        double d; memcpy(&d, &o->f64_bits, sizeof d);
        xpb_encoder_write_float64(enc, d);
        break;
    }
    case OP_STRING:  xpb_encoder_write_string(enc, o->str); break;
    case OP_BYTES:   xpb_encoder_write_bytes(enc, o->bytes, o->bytes_len); break;
    case OP_ARRAY: {
        /* count (int32) then each element */
        xpb_encoder_write_int32(enc, (int32_t)o->n_elems);
        for (size_t i = 0; i < o->n_elems; i++) encode_op(enc, &o->elems[i]);
        break;
    }
    case OP_MAP: {
        xpb_encoder_write_int32(enc, (int32_t)o->n_entries);
        for (size_t i = 0; i < o->n_entries; i++) {
            encode_op(enc, o->entries[i].k);
            encode_op(enc, o->entries[i].v);
        }
        break;
    }
    case OP_MESSAGE: {
        /* length-prefixed nested ops: encode inner to its own buffer, then
         * write as a message (length-prefixed bytes). */
        struct xpb_encoder* inner = xpb_encoder_create(64);
        for (size_t i = 0; i < o->n_ops; i++) encode_op(inner, &o->ops[i]);
        size_t ilen = 0;
        uint8_t* idata = xpb_encoder_finish(inner, &ilen);
        xpb_encoder_write_message(enc, idata, ilen);
        free(idata);
        xpb_encoder_destroy(inner);
        break;
    }
    }
}

/* ----------------------------------------------------------------------- *
 * Decode + verify a sequence of ops against the C decoder.                *
 * ----------------------------------------------------------------------- */

/* Verify a single scalar element of the given type against expected op. */
static bool verify_scalar(struct xpb_decoder* dec, scalar_type t, const op* exp) {
    switch (t) {
    case T_BOOL:    return xpb_decoder_read_bool(dec)   == exp->b;
    case T_INT32:   return xpb_decoder_read_int32(dec)  == exp->i32;
    case T_INT64:   return xpb_decoder_read_int64(dec)  == exp->i64;
    case T_UINT32:  return xpb_decoder_read_uint32(dec) == exp->u32;
    case T_UINT64:  return xpb_decoder_read_uint64(dec) == exp->u64;
    case T_FLOAT32: {
        float f = xpb_decoder_read_float32(dec);
        uint32_t bits; memcpy(&bits, &f, sizeof bits);
        return bits == exp->f32_bits;
    }
    case T_FLOAT64: {
        double d = xpb_decoder_read_float64(dec);
        uint64_t bits; memcpy(&bits, &d, sizeof bits);
        return bits == exp->f64_bits;
    }
    case T_STRING: {
        char* s = xpb_decoder_read_string(dec);
        bool ok = (s != NULL) && (strcmp(s, exp->str) == 0);
        xpb_free(s);
        return ok;
    }
    case T_BYTES: {
        size_t n = 0;
        uint8_t* p = xpb_decoder_read_bytes(dec, &n);
        bool ok = (n == exp->bytes_len) &&
                  (n == 0 || (p != NULL && memcmp(p, exp->bytes, n) == 0));
        xpb_free(p);
        return ok;
    }
    }
    return false;
}

static bool verify_op(struct xpb_decoder* dec, const op* o) {
    switch (o->kind) {
    case OP_BOOL:    return verify_scalar(dec, T_BOOL, o);
    case OP_INT32:   return verify_scalar(dec, T_INT32, o);
    case OP_INT64:   return verify_scalar(dec, T_INT64, o);
    case OP_UINT32:  return verify_scalar(dec, T_UINT32, o);
    case OP_UINT64:  return verify_scalar(dec, T_UINT64, o);
    case OP_FLOAT32: return verify_scalar(dec, T_FLOAT32, o);
    case OP_FLOAT64: return verify_scalar(dec, T_FLOAT64, o);
    case OP_STRING:  return verify_scalar(dec, T_STRING, o);
    case OP_BYTES:   return verify_scalar(dec, T_BYTES, o);
    case OP_ARRAY: {
        int32_t count = xpb_decoder_read_int32(dec);
        if (count < 0 || (size_t)count != o->n_elems) return false;
        for (size_t i = 0; i < o->n_elems; i++) {
            if (!verify_scalar(dec, o->elem_type, &o->elems[i])) return false;
        }
        return true;
    }
    case OP_MAP: {
        int32_t count = xpb_decoder_read_int32(dec);
        if (count < 0 || (size_t)count != o->n_entries) return false;
        for (size_t i = 0; i < o->n_entries; i++) {
            if (!verify_scalar(dec, o->key_type, o->entries[i].k)) return false;
            if (!verify_scalar(dec, o->val_type, o->entries[i].v)) return false;
        }
        return true;
    }
    case OP_MESSAGE: {
        size_t ilen = 0;
        uint8_t* idata = xpb_decoder_read_message_bytes(dec, &ilen);
        struct xpb_decoder* inner = xpb_decoder_create(idata, ilen);
        bool ok = true;
        for (size_t i = 0; i < o->n_ops; i++) {
            if (!verify_op(inner, &o->ops[i])) { ok = false; break; }
        }
        if (!xpb_decoder_ok(inner)) ok = false;
        xpb_decoder_destroy(inner);
        xpb_free(idata);
        return ok;
    }
    }
    return false;
}

/* ----------------------------------------------------------------------- *
 * Path discovery + file IO.                                               *
 * ----------------------------------------------------------------------- */

static bool dir_has_vectors(const char* dir) {
    char path[4096];
    snprintf(path, sizeof path, "%s/vectors.json", dir);
    struct stat st;
    return stat(path, &st) == 0;
}

static bool find_testdata_dir(const char* argv1, char* out, size_t out_sz) {
    if (argv1 && dir_has_vectors(argv1)) {
        snprintf(out, out_sz, "%s", argv1);
        return true;
    }
    const char* env = getenv("XPB_CONFORMANCE_DIR");
    if (env && dir_has_vectors(env)) {
        snprintf(out, out_sz, "%s", env);
        return true;
    }
    /* Walk up from this source file's compiled-in dir. __FILE__ at build is
     * .../tests/c/xpb_conformance.c; testdata is at repo-root/testdata. */
    const char* guesses[] = {
        "testdata/conformance",
        "../../testdata/conformance",
        "../../../testdata/conformance",
        "../testdata/conformance",
        "./testdata/conformance",
    };
    for (size_t i = 0; i < sizeof(guesses) / sizeof(guesses[0]); i++) {
        if (dir_has_vectors(guesses[i])) {
            snprintf(out, out_sz, "%s", guesses[i]);
            return true;
        }
    }
    return false;
}

static uint8_t* read_file(const char* dir, const char* name, size_t* out_len) {
    char path[4096];
    snprintf(path, sizeof path, "%s/%s", dir, name);
    FILE* f = fopen(path, "rb");
    if (!f) return NULL;
    if (fseek(f, 0, SEEK_END) != 0) { fclose(f); return NULL; }
    long sz = ftell(f);
    if (sz < 0) { fclose(f); return NULL; }
    rewind(f);
    uint8_t* buf = (uint8_t*)malloc((size_t)sz + 1); /* +1 so 0-byte file != malloc(0) */
    if (!buf) { fclose(f); return NULL; }
    size_t got = (sz > 0) ? fread(buf, 1, (size_t)sz, f) : 0;
    fclose(f);
    if (got != (size_t)sz) { free(buf); return NULL; }
    *out_len = (size_t)sz;
    return buf;
}

/* ----------------------------------------------------------------------- *
 * Driver.                                                                  *
 * ----------------------------------------------------------------------- */

int main(int argc, char** argv) {
    init_dynamic_vectors();

    char dir[4096];
    if (!find_testdata_dir(argc > 1 ? argv[1] : NULL, dir, sizeof dir)) {
        fprintf(stderr,
            "ERROR: could not locate testdata/conformance. Pass it as argv[1] "
            "or set XPB_CONFORMANCE_DIR.\n");
        return 2;
    }

    printf("===========================================\n");
    printf("XPB V2 C Conformance Harness\n");
    printf("testdata: %s\n", dir);
    printf("vectors:  %zu\n", N_VECTORS);
    printf("===========================================\n");

    int vec_pass = 0;
    int vec_fail = 0;

    for (size_t vi = 0; vi < N_VECTORS; vi++) {
        const vector* vec = &g_vectors[vi];
        size_t golden_len = 0;
        uint8_t* golden = read_file(dir, vec->file, &golden_len);
        if (!golden) {
            printf("  [FAIL] %s: could not read .bin file\n", vec->file);
            g_fail++; vec_fail++;
            continue;
        }

        int before_fail = g_fail;

        /* 1. Decode golden bytes and verify decoded values. */
        struct xpb_decoder* dec = xpb_decoder_create(golden, golden_len);
        for (size_t i = 0; i < vec->n_ops; i++) {
            char nm[256];
            snprintf(nm, sizeof nm, "%s decode op[%zu]", vec->file, i);
            CHECK(nm, verify_op(dec, &vec->ops[i]));
        }
        {
            char nm[256];
            snprintf(nm, sizeof nm, "%s decoder ok + consumed", vec->file);
            CHECK(nm, xpb_decoder_ok(dec) && xpb_decoder_eof(dec));
        }
        xpb_decoder_destroy(dec);

        /* 2. Re-encode and assert byte-identical to golden. */
        struct xpb_encoder* enc = xpb_encoder_create(64);
        for (size_t i = 0; i < vec->n_ops; i++) encode_op(enc, &vec->ops[i]);
        size_t enc_len = 0;
        uint8_t* enc_data = xpb_encoder_finish(enc, &enc_len);

        char nm[256];
        snprintf(nm, sizeof nm, "%s re-encode length", vec->file);
        CHECK(nm, enc_len == golden_len);

        snprintf(nm, sizeof nm, "%s re-encode byte-identical", vec->file);
        bool identical = (enc_len == golden_len) &&
                         (golden_len == 0 ||
                          (enc_data != NULL && memcmp(enc_data, golden, golden_len) == 0));
        CHECK(nm, identical);
        if (!identical) {
            printf("        expected %zu bytes, got %zu bytes\n", golden_len, enc_len);
        }

        free(enc_data);
        xpb_encoder_destroy(enc);
        free(golden);

        if (g_fail == before_fail) vec_pass++;
        else vec_fail++;
    }

    printf("\n===========================================\n");
    printf("Vectors: %d passed, %d failed (of %zu)\n", vec_pass, vec_fail, N_VECTORS);
    printf("Checks:  %d passed, %d failed\n", g_pass, g_fail);
    printf("===========================================\n");

    return g_fail > 0 ? 1 : 0;
}
