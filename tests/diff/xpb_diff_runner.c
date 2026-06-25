/*
 * Cross-language DIFFERENTIAL runner for the C runtime (T-9).
 *
 * This is the C arm of the random cross-language differential fuzzer in
 * tests/differential. Unlike the committed conformance harness
 * (tests/c/xpb_conformance.c) -- which hardcodes the expected op table keyed by
 * a FIXED filename set -- this runner is fully DATA-DRIVEN: it parses the
 * vectors.json manifest at runtime, so the Go driver can feed it an arbitrary
 * random corpus. The committed harness is left untouched.
 *
 * Usage:  xpb_diff_runner <corpus-dir> [bytes|values]
 *
 * For each vector it decodes the .bin with the C runtime and verifies decoded
 * values bit-exactly against the manifest ops. In `bytes` mode (default; the
 * map-FREE corpus) it then re-encodes and asserts byte-identity with the Go
 * reference .bin. In `values` mode (the map-CONTAINING corpus) the byte-identity
 * check is skipped, because map entry order is non-canonical across runtimes
 * (T-7); the decoded values are still a real cross-language oracle. Returns
 * non-zero on any mismatch. Compiled under ASan/UBSan by the Go driver.
 *
 * Scope note: the manifest value model (see tests/conformance/manifest.go) keeps
 * arrays and maps homogeneous over scalar element types; this parser therefore
 * only needs scalars + array/map/message composites, which is exactly what the
 * differential generator emits. Manifest strings never contain embedded NULs
 * (the generator emits printable ASCII + multibyte runes only), so a
 * NUL-terminated C string round-trips losslessly.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <inttypes.h>
#include <xpb/xpb.h>

/* ----------------------------------------------------------------------- *
 * Minimal JSON value model + parser (objects, arrays, strings, numbers,    *
 * bool, null). Self-contained; handles the escapes/UTF-8 the manifest uses.*
 * ----------------------------------------------------------------------- */

typedef enum { J_NULL, J_BOOL, J_NUM, J_STR, J_ARR, J_OBJ } jkind;

typedef struct jval jval;
typedef struct {
    char* key;
    jval* val;
} jmember;

struct jval {
    jkind kind;
    bool b;
    double num;
    char* str;      /* NUL-terminated, decoded UTF-8 */
    size_t str_len; /* byte length (excludes terminator) */
    jval** items;   /* array elements */
    size_t n_items;
    jmember* members; /* object members */
    size_t n_members;
};

/* Depth cap for the recursive descent parser: guards against stack exhaustion
 * on a pathologically nested manifest. The harness only ever emits shallow
 * nesting (genConfig.maxDepth=4), so this is defense-in-depth. */
#define JSON_MAX_DEPTH 256

typedef struct {
    const char* s;
    size_t pos;
    size_t len;
    int depth;
    bool err;
} jparser;

static void jp_fail(jparser* p) { p->err = true; }

static jval* jv_new(jkind k) {
    jval* v = (jval*)calloc(1, sizeof(jval));
    if (v) v->kind = k;
    return v;
}

static void jv_free(jval* v) {
    if (!v) return;
    free(v->str);
    for (size_t i = 0; i < v->n_items; i++) jv_free(v->items[i]);
    free(v->items);
    for (size_t i = 0; i < v->n_members; i++) {
        free(v->members[i].key);
        jv_free(v->members[i].val);
    }
    free(v->members);
    free(v);
}

static void jp_skip_ws(jparser* p) {
    while (p->pos < p->len) {
        char c = p->s[p->pos];
        if (c == ' ' || c == '\t' || c == '\n' || c == '\r') p->pos++;
        else break;
    }
}

static jval* jp_value(jparser* p);

/* Append a single byte to a growable buffer. */
static bool buf_push(char** buf, size_t* len, size_t* cap, char c) {
    if (*len + 1 >= *cap) {
        size_t nc = *cap ? *cap * 2 : 16;
        char* nb = (char*)realloc(*buf, nc);
        if (!nb) return false;
        *buf = nb;
        *cap = nc;
    }
    (*buf)[(*len)++] = c;
    return true;
}

/* Encode a Unicode code point as UTF-8 into the buffer. */
static bool utf8_push(char** buf, size_t* len, size_t* cap, unsigned cp) {
    if (cp <= 0x7F) {
        return buf_push(buf, len, cap, (char)cp);
    } else if (cp <= 0x7FF) {
        return buf_push(buf, len, cap, (char)(0xC0 | (cp >> 6))) &&
               buf_push(buf, len, cap, (char)(0x80 | (cp & 0x3F)));
    } else if (cp <= 0xFFFF) {
        return buf_push(buf, len, cap, (char)(0xE0 | (cp >> 12))) &&
               buf_push(buf, len, cap, (char)(0x80 | ((cp >> 6) & 0x3F))) &&
               buf_push(buf, len, cap, (char)(0x80 | (cp & 0x3F)));
    } else {
        return buf_push(buf, len, cap, (char)(0xF0 | (cp >> 18))) &&
               buf_push(buf, len, cap, (char)(0x80 | ((cp >> 12) & 0x3F))) &&
               buf_push(buf, len, cap, (char)(0x80 | ((cp >> 6) & 0x3F))) &&
               buf_push(buf, len, cap, (char)(0x80 | (cp & 0x3F)));
    }
}

static int hex4(jparser* p) {
    int v = 0;
    for (int i = 0; i < 4; i++) {
        if (p->pos >= p->len) { jp_fail(p); return -1; }
        char c = p->s[p->pos++];
        int d;
        if (c >= '0' && c <= '9') d = c - '0';
        else if (c >= 'a' && c <= 'f') d = c - 'a' + 10;
        else if (c >= 'A' && c <= 'F') d = c - 'A' + 10;
        else { jp_fail(p); return -1; }
        v = (v << 4) | d;
    }
    return v;
}

static jval* jp_string(jparser* p) {
    /* assumes current char is the opening quote */
    p->pos++; /* consume " */
    char* buf = NULL;
    size_t len = 0, cap = 0;
    while (p->pos < p->len) {
        char c = p->s[p->pos++];
        if (c == '"') {
            if (!buf_push(&buf, &len, &cap, '\0')) { jp_fail(p); free(buf); return NULL; }
            jval* v = jv_new(J_STR);
            if (!v) { free(buf); jp_fail(p); return NULL; }
            v->str = buf;
            v->str_len = len - 1;
            return v;
        }
        if (c == '\\') {
            if (p->pos >= p->len) { jp_fail(p); free(buf); return NULL; }
            char e = p->s[p->pos++];
            bool ok = true;
            switch (e) {
            case '"':  ok = buf_push(&buf, &len, &cap, '"'); break;
            case '\\': ok = buf_push(&buf, &len, &cap, '\\'); break;
            case '/':  ok = buf_push(&buf, &len, &cap, '/'); break;
            case 'b':  ok = buf_push(&buf, &len, &cap, '\b'); break;
            case 'f':  ok = buf_push(&buf, &len, &cap, '\f'); break;
            case 'n':  ok = buf_push(&buf, &len, &cap, '\n'); break;
            case 'r':  ok = buf_push(&buf, &len, &cap, '\r'); break;
            case 't':  ok = buf_push(&buf, &len, &cap, '\t'); break;
            case 'u': {
                int cp = hex4(p);
                if (cp < 0) { free(buf); return NULL; }
                if (cp >= 0xD800 && cp <= 0xDBFF) {
                    /* high surrogate: expect \uXXXX low surrogate */
                    if (p->pos + 1 >= p->len || p->s[p->pos] != '\\' || p->s[p->pos + 1] != 'u') {
                        jp_fail(p); free(buf); return NULL;
                    }
                    p->pos += 2;
                    int lo = hex4(p);
                    if (lo < 0) { free(buf); return NULL; }
                    /* Reject a low surrogate outside [0xDC00,0xDFFF]: composing
                     * one would underflow ((unsigned)lo - 0xDC00) and emit garbage. */
                    if (lo < 0xDC00 || lo > 0xDFFF) { jp_fail(p); free(buf); return NULL; }
                    unsigned full = 0x10000 + (((unsigned)cp - 0xD800) << 10) + ((unsigned)lo - 0xDC00);
                    ok = utf8_push(&buf, &len, &cap, full);
                } else {
                    ok = utf8_push(&buf, &len, &cap, (unsigned)cp);
                }
                break;
            }
            default: jp_fail(p); free(buf); return NULL;
            }
            if (!ok) { jp_fail(p); free(buf); return NULL; }
        } else {
            if (!buf_push(&buf, &len, &cap, c)) { jp_fail(p); free(buf); return NULL; }
        }
    }
    jp_fail(p);
    free(buf);
    return NULL;
}

static jval* jp_number(jparser* p) {
    size_t start = p->pos;
    while (p->pos < p->len) {
        char c = p->s[p->pos];
        if ((c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' ||
            c == 'e' || c == 'E') {
            p->pos++;
        } else break;
    }
    char tmp[64];
    size_t n = p->pos - start;
    if (n == 0 || n >= sizeof tmp) { jp_fail(p); return NULL; }
    memcpy(tmp, p->s + start, n);
    tmp[n] = '\0';
    jval* v = jv_new(J_NUM);
    if (!v) { jp_fail(p); return NULL; }
    v->num = strtod(tmp, NULL);
    return v;
}

static jval* jp_array(jparser* p) {
    p->pos++; /* [ */
    jval* v = jv_new(J_ARR);
    if (!v) { jp_fail(p); return NULL; }
    jp_skip_ws(p);
    if (p->pos < p->len && p->s[p->pos] == ']') { p->pos++; return v; }
    for (;;) {
        jval* item = jp_value(p);
        if (p->err) { jv_free(item); jv_free(v); return NULL; }
        /* Overflow guard on the growth multiply (defense-in-depth; the file
         * length already bounds n_items in practice). */
        if (v->n_items + 1 > SIZE_MAX / sizeof(jval*)) {
            jp_fail(p); jv_free(item); jv_free(v); return NULL;
        }
        jval** ni = (jval**)realloc(v->items, (v->n_items + 1) * sizeof(jval*));
        if (!ni) { jp_fail(p); jv_free(item); jv_free(v); return NULL; }
        v->items = ni;
        v->items[v->n_items++] = item;
        jp_skip_ws(p);
        if (p->pos >= p->len) { jp_fail(p); jv_free(v); return NULL; }
        char c = p->s[p->pos++];
        if (c == ']') break;
        if (c != ',') { jp_fail(p); jv_free(v); return NULL; }
        jp_skip_ws(p);
    }
    return v;
}

static jval* jp_object(jparser* p) {
    p->pos++; /* { */
    jval* v = jv_new(J_OBJ);
    if (!v) { jp_fail(p); return NULL; }
    jp_skip_ws(p);
    if (p->pos < p->len && p->s[p->pos] == '}') { p->pos++; return v; }
    for (;;) {
        jp_skip_ws(p);
        if (p->pos >= p->len || p->s[p->pos] != '"') { jp_fail(p); jv_free(v); return NULL; }
        jval* key = jp_string(p);
        if (p->err) { jv_free(key); jv_free(v); return NULL; }
        jp_skip_ws(p);
        if (p->pos >= p->len || p->s[p->pos++] != ':') { jp_fail(p); jv_free(key); jv_free(v); return NULL; }
        jp_skip_ws(p);
        jval* val = jp_value(p);
        if (p->err) { jv_free(key); jv_free(val); jv_free(v); return NULL; }
        if (v->n_members + 1 > SIZE_MAX / sizeof(jmember)) {
            jp_fail(p); jv_free(key); jv_free(val); jv_free(v); return NULL;
        }
        jmember* nm = (jmember*)realloc(v->members, (v->n_members + 1) * sizeof(jmember));
        if (!nm) { jp_fail(p); jv_free(key); jv_free(val); jv_free(v); return NULL; }
        v->members = nm;
        v->members[v->n_members].key = key->str; /* steal */
        key->str = NULL;
        jv_free(key);
        v->members[v->n_members].val = val;
        v->n_members++;
        jp_skip_ws(p);
        if (p->pos >= p->len) { jp_fail(p); jv_free(v); return NULL; }
        char c = p->s[p->pos++];
        if (c == '}') break;
        if (c != ',') { jp_fail(p); jv_free(v); return NULL; }
    }
    return v;
}

static jval* jp_value(jparser* p) {
    jp_skip_ws(p);
    if (p->pos >= p->len) { jp_fail(p); return NULL; }
    char c = p->s[p->pos];
    switch (c) {
    case '"': return jp_string(p);
    case '{':
    case '[':
        if (p->depth >= JSON_MAX_DEPTH) { jp_fail(p); return NULL; }
        p->depth++;
        {
            jval* v = (c == '{') ? jp_object(p) : jp_array(p);
            p->depth--;
            return v;
        }
    case 't':
        if (p->pos + 4 <= p->len && memcmp(p->s + p->pos, "true", 4) == 0) {
            p->pos += 4; jval* v = jv_new(J_BOOL); if (v) v->b = true; else jp_fail(p); return v;
        }
        jp_fail(p); return NULL;
    case 'f':
        if (p->pos + 5 <= p->len && memcmp(p->s + p->pos, "false", 5) == 0) {
            p->pos += 5; jval* v = jv_new(J_BOOL); if (v) v->b = false; else jp_fail(p); return v;
        }
        jp_fail(p); return NULL;
    case 'n':
        if (p->pos + 4 <= p->len && memcmp(p->s + p->pos, "null", 4) == 0) {
            p->pos += 4; return jv_new(J_NULL);
        }
        jp_fail(p); return NULL;
    default:
        return jp_number(p);
    }
}

static jval* json_parse(const char* s, size_t len) {
    jparser p = { s, 0, len, 0, false };
    jval* v = jp_value(&p);
    if (p.err) { jv_free(v); return NULL; }
    return v;
}

/* Object member lookup. */
static jval* jobj_get(const jval* o, const char* key) {
    if (!o || o->kind != J_OBJ) return NULL;
    for (size_t i = 0; i < o->n_members; i++) {
        if (strcmp(o->members[i].key, key) == 0) return o->members[i].val;
    }
    return NULL;
}

static const char* jstr(const jval* o, const char* key, const char* dflt) {
    jval* v = jobj_get(o, key);
    return (v && v->kind == J_STR) ? v->str : dflt;
}

/* ----------------------------------------------------------------------- *
 * hex helpers + bit-pattern parsing                                        *
 * ----------------------------------------------------------------------- */

static int hexnib(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

/* Decode a lowercase hex string into a freshly-malloc'd byte buffer. */
static uint8_t* hex_decode(const char* s, size_t* out_len) {
    size_t n = s ? strlen(s) : 0;
    *out_len = n / 2;
    uint8_t* out = (uint8_t*)malloc(*out_len ? *out_len : 1);
    if (!out) return NULL;
    for (size_t i = 0; i + 1 < n; i += 2) {
        out[i / 2] = (uint8_t)((hexnib(s[i]) << 4) | hexnib(s[i + 1]));
    }
    return out;
}

static const char* strip_hex_prefix(const char* s) {
    if (s && (strncmp(s, "0x", 2) == 0 || strncmp(s, "0X", 2) == 0)) return s + 2;
    return s;
}

static uint32_t parse_u32_hex(const char* s) {
    return (uint32_t)strtoul(strip_hex_prefix(s), NULL, 16);
}
static uint64_t parse_u64_hex(const char* s) {
    return (uint64_t)strtoull(strip_hex_prefix(s), NULL, 16);
}

/* ----------------------------------------------------------------------- *
 * Encode + verify ops from JSON.                                           *
 * ----------------------------------------------------------------------- */

static int g_fail = 0;
#define VFAIL(name, msg) do { printf("  [FAIL] %s: %s\n", (name), (msg)); g_fail++; } while (0)

static void encode_op(struct xpb_encoder* enc, const jval* op);

static void encode_ops_arr(struct xpb_encoder* enc, const jval* ops) {
    if (!ops || ops->kind != J_ARR) return;
    for (size_t i = 0; i < ops->n_items; i++) encode_op(enc, ops->items[i]);
}

static void encode_op(struct xpb_encoder* enc, const jval* op) {
    const char* ty = jstr(op, "type", "");
    if (strcmp(ty, "bool") == 0) {
        jval* v = jobj_get(op, "bool");
        xpb_encoder_write_bool(enc, v && v->kind == J_BOOL ? v->b : false);
    } else if (strcmp(ty, "int32") == 0) {
        jval* v = jobj_get(op, "int32");
        xpb_encoder_write_int32(enc, v ? (int32_t)(int64_t)v->num : 0);
    } else if (strcmp(ty, "uint32") == 0) {
        jval* v = jobj_get(op, "uint32");
        xpb_encoder_write_uint32(enc, v ? (uint32_t)(int64_t)v->num : 0);
    } else if (strcmp(ty, "int64") == 0) {
        const char* s = jstr(op, "int64", "0");
        xpb_encoder_write_int64(enc, (int64_t)strtoll(s, NULL, 10));
    } else if (strcmp(ty, "uint64") == 0) {
        const char* s = jstr(op, "uint64", "0");
        xpb_encoder_write_uint64(enc, (uint64_t)strtoull(s, NULL, 10));
    } else if (strcmp(ty, "float32") == 0) {
        uint32_t bits = parse_u32_hex(jstr(op, "floatBits", "0"));
        float f; memcpy(&f, &bits, sizeof f);
        xpb_encoder_write_float32(enc, f);
    } else if (strcmp(ty, "float64") == 0) {
        uint64_t bits = parse_u64_hex(jstr(op, "floatBits", "0"));
        double d; memcpy(&d, &bits, sizeof d);
        xpb_encoder_write_float64(enc, d);
    } else if (strcmp(ty, "string") == 0) {
        xpb_encoder_write_string(enc, jstr(op, "string", ""));
    } else if (strcmp(ty, "bytes") == 0) {
        size_t n; uint8_t* b = hex_decode(jstr(op, "bytes", ""), &n);
        xpb_encoder_write_bytes(enc, b, n);
        free(b);
    } else if (strcmp(ty, "array") == 0) {
        jval* elems = jobj_get(op, "elems");
        size_t n = (elems && elems->kind == J_ARR) ? elems->n_items : 0;
        xpb_encoder_write_int32(enc, (int32_t)n);
        for (size_t i = 0; i < n; i++) encode_op(enc, elems->items[i]);
    } else if (strcmp(ty, "map") == 0) {
        jval* ents = jobj_get(op, "entries");
        size_t n = (ents && ents->kind == J_ARR) ? ents->n_items : 0;
        xpb_encoder_write_int32(enc, (int32_t)n);
        for (size_t i = 0; i < n; i++) {
            encode_op(enc, jobj_get(ents->items[i], "k"));
            encode_op(enc, jobj_get(ents->items[i], "v"));
        }
    } else if (strcmp(ty, "message") == 0) {
        struct xpb_encoder* inner = xpb_encoder_create(64);
        if (!inner) return; /* allocation failure: leave enc untouched */
        encode_ops_arr(inner, jobj_get(op, "ops"));
        size_t ilen = 0;
        uint8_t* idata = xpb_encoder_finish(inner, &ilen);
        xpb_encoder_write_message(enc, idata, ilen);
        free(idata);
        xpb_encoder_destroy(inner);
    }
}

static bool verify_op(struct xpb_decoder* dec, const jval* op, const char* name);

static bool verify_ops_arr(struct xpb_decoder* dec, const jval* ops, const char* name) {
    if (!ops || ops->kind != J_ARR) return true;
    for (size_t i = 0; i < ops->n_items; i++) {
        if (!verify_op(dec, ops->items[i], name)) return false;
    }
    return true;
}

static bool verify_op(struct xpb_decoder* dec, const jval* op, const char* name) {
    const char* ty = jstr(op, "type", "");
    if (strcmp(ty, "bool") == 0) {
        jval* v = jobj_get(op, "bool");
        return xpb_decoder_read_bool(dec) == (v && v->kind == J_BOOL ? v->b : false);
    } else if (strcmp(ty, "int32") == 0) {
        jval* v = jobj_get(op, "int32");
        return xpb_decoder_read_int32(dec) == (v ? (int32_t)(int64_t)v->num : 0);
    } else if (strcmp(ty, "uint32") == 0) {
        jval* v = jobj_get(op, "uint32");
        return xpb_decoder_read_uint32(dec) == (v ? (uint32_t)(int64_t)v->num : 0);
    } else if (strcmp(ty, "int64") == 0) {
        return xpb_decoder_read_int64(dec) == (int64_t)strtoll(jstr(op, "int64", "0"), NULL, 10);
    } else if (strcmp(ty, "uint64") == 0) {
        return xpb_decoder_read_uint64(dec) == (uint64_t)strtoull(jstr(op, "uint64", "0"), NULL, 10);
    } else if (strcmp(ty, "float32") == 0) {
        float f = xpb_decoder_read_float32(dec);
        uint32_t bits; memcpy(&bits, &f, sizeof bits);
        return bits == parse_u32_hex(jstr(op, "floatBits", "0"));
    } else if (strcmp(ty, "float64") == 0) {
        double d = xpb_decoder_read_float64(dec);
        uint64_t bits; memcpy(&bits, &d, sizeof bits);
        return bits == parse_u64_hex(jstr(op, "floatBits", "0"));
    } else if (strcmp(ty, "string") == 0) {
        char* s = xpb_decoder_read_string(dec);
        bool ok = (s != NULL) && strcmp(s, jstr(op, "string", "")) == 0;
        xpb_free(s);
        return ok;
    } else if (strcmp(ty, "bytes") == 0) {
        size_t want_n; uint8_t* want = hex_decode(jstr(op, "bytes", ""), &want_n);
        size_t got_n = 0; uint8_t* got = xpb_decoder_read_bytes(dec, &got_n);
        bool ok = (got_n == want_n) && (got_n == 0 || (got && memcmp(got, want, got_n) == 0));
        xpb_free(got);
        free(want);
        return ok;
    } else if (strcmp(ty, "array") == 0) {
        jval* elems = jobj_get(op, "elems");
        size_t n = (elems && elems->kind == J_ARR) ? elems->n_items : 0;
        int32_t count = xpb_decoder_read_int32(dec);
        if (count < 0 || (size_t)count != n) return false;
        for (size_t i = 0; i < n; i++) {
            if (!verify_op(dec, elems->items[i], name)) return false;
        }
        return true;
    } else if (strcmp(ty, "map") == 0) {
        jval* ents = jobj_get(op, "entries");
        size_t n = (ents && ents->kind == J_ARR) ? ents->n_items : 0;
        int32_t count = xpb_decoder_read_int32(dec);
        if (count < 0 || (size_t)count != n) return false;
        for (size_t i = 0; i < n; i++) {
            if (!verify_op(dec, jobj_get(ents->items[i], "k"), name)) return false;
            if (!verify_op(dec, jobj_get(ents->items[i], "v"), name)) return false;
        }
        return true;
    } else if (strcmp(ty, "message") == 0) {
        size_t ilen = 0;
        uint8_t* idata = xpb_decoder_read_message_bytes(dec, &ilen);
        struct xpb_decoder* inner = xpb_decoder_create(idata, ilen);
        bool ok = verify_ops_arr(inner, jobj_get(op, "ops"), name);
        if (!xpb_decoder_ok(inner) || !xpb_decoder_eof(inner)) ok = false;
        xpb_decoder_destroy(inner);
        xpb_free(idata);
        return ok;
    }
    return false;
}

/* ----------------------------------------------------------------------- *
 * file IO + driver                                                         *
 * ----------------------------------------------------------------------- */

static char* read_text(const char* path, size_t* out_len) {
    FILE* f = fopen(path, "rb");
    if (!f) return NULL;
    if (fseek(f, 0, SEEK_END) != 0) { fclose(f); return NULL; }
    long sz = ftell(f);
    if (sz < 0) { fclose(f); return NULL; }
    rewind(f);
    char* buf = (char*)malloc((size_t)sz + 1);
    if (!buf) { fclose(f); return NULL; }
    size_t got = (sz > 0) ? fread(buf, 1, (size_t)sz, f) : 0;
    fclose(f);
    if (got != (size_t)sz) { free(buf); return NULL; }
    buf[sz] = '\0';
    *out_len = (size_t)sz;
    return buf;
}

int main(int argc, char** argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: %s <corpus-dir> [bytes|values]\n", argv[0]);
        return 2;
    }
    const char* dir = argv[1];
    bool values_only = (argc >= 3 && strcmp(argv[2], "values") == 0);

    char manifest_path[4096];
    snprintf(manifest_path, sizeof manifest_path, "%s/vectors.json", dir);
    size_t mlen = 0;
    char* raw = read_text(manifest_path, &mlen);
    if (!raw) {
        fprintf(stderr, "ERROR: cannot read %s\n", manifest_path);
        return 2;
    }
    jval* manifest = json_parse(raw, mlen);
    free(raw);
    if (!manifest) {
        fprintf(stderr, "ERROR: cannot parse %s\n", manifest_path);
        return 2;
    }
    jval* vectors = jobj_get(manifest, "vectors");
    if (!vectors || vectors->kind != J_ARR || vectors->n_items == 0) {
        fprintf(stderr, "ERROR: manifest has no vectors\n");
        jv_free(manifest);
        return 1;
    }

    size_t n_ok = 0;
    for (size_t vi = 0; vi < vectors->n_items; vi++) {
        const jval* v = vectors->items[vi];
        const char* name = jstr(v, "name", "?");
        const char* file = jstr(v, "file", NULL);
        jval* ops = jobj_get(v, "ops");
        if (!file) { VFAIL(name, "missing file field"); continue; }

        char bin_path[4096];
        snprintf(bin_path, sizeof bin_path, "%s/%s", dir, file);
        size_t glen = 0;
        char* gtext = read_text(bin_path, &glen);
        if (!gtext) { VFAIL(name, "cannot read .bin"); continue; }
        uint8_t* golden = (uint8_t*)gtext;

        /* 1. Decode + verify values. */
        struct xpb_decoder* dec = xpb_decoder_create(golden, glen);
        bool ok = verify_ops_arr(dec, ops, name);
        if (!xpb_decoder_ok(dec) || !xpb_decoder_eof(dec)) ok = false;
        xpb_decoder_destroy(dec);
        if (!ok) { VFAIL(name, "decode/verify mismatch"); free(gtext); continue; }

        /* 2. Re-encode + assert byte-identity (byte mode only; skipped for the
         *    map-containing corpus, where order is non-canonical). */
        if (!values_only) {
            struct xpb_encoder* enc = xpb_encoder_create(256);
            encode_ops_arr(enc, ops);
            size_t elen = 0;
            uint8_t* edata = xpb_encoder_finish(enc, &elen);
            bool identical = (elen == glen) &&
                             (glen == 0 || (edata && memcmp(edata, golden, glen) == 0));
            if (!identical) {
                char msg[128];
                snprintf(msg, sizeof msg, "re-encode mismatch (got %zu bytes, want %zu)", elen, glen);
                VFAIL(name, msg);
            } else {
                n_ok++;
            }
            free(edata);
            xpb_encoder_destroy(enc);
        } else {
            /* values mode (map-containing corpus): we cannot assert byte-identity
             * (map order is non-canonical), but we DO exercise this runtime's
             * ENCODER by re-encoding the values and decoding THAT back, asserting
             * the values survive the runtime's own encode->decode round-trip. */
            struct xpb_encoder* enc = xpb_encoder_create(256);
            if (enc) {
                encode_ops_arr(enc, ops);
                size_t elen = 0;
                uint8_t* edata = xpb_encoder_finish(enc, &elen);
                struct xpb_decoder* rdec = xpb_decoder_create(edata, elen);
                bool rok = verify_ops_arr(rdec, ops, name);
                if (!xpb_decoder_ok(rdec) || !xpb_decoder_eof(rdec)) rok = false;
                xpb_decoder_destroy(rdec);
                if (!rok) {
                    VFAIL(name, "self round-trip (encode->decode) value mismatch");
                } else {
                    n_ok++;
                }
                free(edata);
                xpb_encoder_destroy(enc);
            } else {
                VFAIL(name, "encoder alloc failed");
            }
        }
        free(gtext);
    }

    jv_free(manifest);

    printf("C differential (%s): %zu vectors verified, %d failed\n",
           values_only ? "values" : "bytes", n_ok, g_fail);
    return g_fail > 0 ? 1 : 0;
}
