/*
 * Timed cross-runtime benchmark harness for the C runtime (xpbench / T-17).
 *
 * This is the C arm of the cross-runtime benchmark TABLE driven by cmd/xpbench.
 * It is the timed analogue of the proven differential runner
 * (tests/diff/xpb_diff_runner.c): a fully data-driven harness that parses the
 * shared vectors.json manifest + .bin corpus the Go reference encoder wrote,
 * then for every vector times an encode loop (re-encode the ops with the C
 * encoder) and a decode loop (decode + verify the .bin bytes with the C
 * decoder) over a per-vector iteration count, and prints a JSON array of
 * {name, encodeNs, decodeNs, wireSize} to stdout for the Go driver to normalize
 * into table rows.
 *
 * Usage:  c_bench <corpus-dir>
 *
 * Built at -O2 (a benchmark build, no sanitizers) against runtime/c by the Go
 * driver. The JSON parser + op encode/verify are lifted from the differential
 * runner; only the driver (timing loops + JSON emit) differs.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <inttypes.h>
#include <time.h>
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
    char* str;
    size_t str_len;
    jval** items;
    size_t n_items;
    jmember* members;
    size_t n_members;
};

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
                    if (p->pos + 1 >= p->len || p->s[p->pos] != '\\' || p->s[p->pos + 1] != 'u') {
                        jp_fail(p); free(buf); return NULL;
                    }
                    p->pos += 2;
                    int lo = hex4(p);
                    if (lo < 0) { free(buf); return NULL; }
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
    p->pos++;
    jval* v = jv_new(J_ARR);
    if (!v) { jp_fail(p); return NULL; }
    jp_skip_ws(p);
    if (p->pos < p->len && p->s[p->pos] == ']') { p->pos++; return v; }
    for (;;) {
        jval* item = jp_value(p);
        if (p->err) { jv_free(item); jv_free(v); return NULL; }
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
    p->pos++;
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
        v->members[v->n_members].key = key->str;
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
 * Encode + decode/verify ops (the timed work).                             *
 * ----------------------------------------------------------------------- */

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
        if (!inner) return;
        encode_ops_arr(inner, jobj_get(op, "ops"));
        size_t ilen = 0;
        uint8_t* idata = xpb_encoder_finish(inner, &ilen);
        xpb_encoder_write_message(enc, idata, ilen);
        free(idata);
        xpb_encoder_destroy(inner);
    }
}

static uint64_t decode_op(struct xpb_decoder* dec, const jval* op);

static uint64_t decode_ops_arr(struct xpb_decoder* dec, const jval* ops) {
    uint64_t acc = 0;
    if (!ops || ops->kind != J_ARR) return acc;
    for (size_t i = 0; i < ops->n_items; i++) acc += decode_op(dec, ops->items[i]);
    return acc;
}

/* decode_op reads every value (decode-only -- it does NOT verify them, so the
 * decode column is consistent across runtimes) and returns a small accumulator
 * so the optimizer cannot elide the decode in the timed loop. */
static uint64_t decode_op(struct xpb_decoder* dec, const jval* op) {
    const char* ty = jstr(op, "type", "");
    if (strcmp(ty, "bool") == 0) {
        return xpb_decoder_read_bool(dec) ? 1 : 0;
    } else if (strcmp(ty, "int32") == 0) {
        return (uint64_t)(uint32_t)xpb_decoder_read_int32(dec);
    } else if (strcmp(ty, "uint32") == 0) {
        return xpb_decoder_read_uint32(dec);
    } else if (strcmp(ty, "int64") == 0) {
        return (uint64_t)xpb_decoder_read_int64(dec);
    } else if (strcmp(ty, "uint64") == 0) {
        return xpb_decoder_read_uint64(dec);
    } else if (strcmp(ty, "float32") == 0) {
        float f = xpb_decoder_read_float32(dec);
        uint32_t bits; memcpy(&bits, &f, sizeof bits);
        return bits;
    } else if (strcmp(ty, "float64") == 0) {
        double d = xpb_decoder_read_float64(dec);
        uint64_t bits; memcpy(&bits, &d, sizeof bits);
        return bits;
    } else if (strcmp(ty, "string") == 0) {
        char* s = xpb_decoder_read_string(dec);
        uint64_t r = s ? (uint64_t)strlen(s) : 0;
        xpb_free(s);
        return r;
    } else if (strcmp(ty, "bytes") == 0) {
        size_t got_n = 0; uint8_t* got = xpb_decoder_read_bytes(dec, &got_n);
        xpb_free(got);
        return got_n;
    } else if (strcmp(ty, "array") == 0) {
        jval* elems = jobj_get(op, "elems");
        size_t n = (elems && elems->kind == J_ARR) ? elems->n_items : 0;
        int32_t count = xpb_decoder_read_int32(dec);
        uint64_t acc = (uint64_t)(uint32_t)count;
        for (size_t i = 0; i < n; i++) acc += decode_op(dec, elems->items[i]);
        return acc;
    } else if (strcmp(ty, "map") == 0) {
        jval* ents = jobj_get(op, "entries");
        size_t n = (ents && ents->kind == J_ARR) ? ents->n_items : 0;
        int32_t count = xpb_decoder_read_int32(dec);
        uint64_t acc = (uint64_t)(uint32_t)count;
        for (size_t i = 0; i < n; i++) {
            acc += decode_op(dec, jobj_get(ents->items[i], "k"));
            acc += decode_op(dec, jobj_get(ents->items[i], "v"));
        }
        return acc;
    } else if (strcmp(ty, "message") == 0) {
        size_t ilen = 0;
        uint8_t* idata = xpb_decoder_read_message_bytes(dec, &ilen);
        struct xpb_decoder* inner = xpb_decoder_create(idata, ilen);
        uint64_t acc = decode_ops_arr(inner, jobj_get(op, "ops"));
        xpb_decoder_destroy(inner);
        xpb_free(idata);
        return acc + ilen;
    }
    return 0;
}

/* ----------------------------------------------------------------------- *
 * file IO + timing + driver                                                *
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

static double now_ns(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (double)ts.tv_sec * 1e9 + (double)ts.tv_nsec;
}

int main(int argc, char** argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: %s <corpus-dir>\n", argv[0]);
        return 2;
    }
    const char* dir = argv[1];

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

    volatile uint64_t sink = 0;
    printf("[");
    for (size_t vi = 0; vi < vectors->n_items; vi++) {
        const jval* v = vectors->items[vi];
        const char* name = jstr(v, "name", "?");
        const char* file = jstr(v, "file", NULL);
        jval* ops = jobj_get(v, "ops");
        jval* itj = jobj_get(v, "iters");
        long iters = (itj && itj->kind == J_NUM) ? (long)itj->num : 1;
        if (iters < 1) iters = 1;
        if (!file) { fprintf(stderr, "ERROR: vector %s missing file\n", name); continue; }

        char bin_path[4096];
        snprintf(bin_path, sizeof bin_path, "%s/%s", dir, file);
        size_t glen = 0;
        char* gtext = read_text(bin_path, &glen);
        if (!gtext) { fprintf(stderr, "ERROR: cannot read %s\n", bin_path); continue; }
        uint8_t* golden = (uint8_t*)gtext;
        long warm = iters / 10;
        if (warm < 1) warm = 1;

        /* Encode timing. */
        for (long i = 0; i < warm; i++) {
            struct xpb_encoder* enc = xpb_encoder_create(256);
            encode_ops_arr(enc, ops);
            size_t elen = 0;
            uint8_t* edata = xpb_encoder_finish(enc, &elen);
            sink += elen;
            free(edata);
            xpb_encoder_destroy(enc);
        }
        double t0 = now_ns();
        for (long i = 0; i < iters; i++) {
            struct xpb_encoder* enc = xpb_encoder_create(256);
            encode_ops_arr(enc, ops);
            size_t elen = 0;
            uint8_t* edata = xpb_encoder_finish(enc, &elen);
            sink += elen;
            free(edata);
            xpb_encoder_destroy(enc);
        }
        double enc_ns = (now_ns() - t0) / (double)iters;

        /* Decode timing. */
        for (long i = 0; i < warm; i++) {
            struct xpb_decoder* dec = xpb_decoder_create(golden, glen);
            sink += decode_ops_arr(dec, ops);
            xpb_decoder_destroy(dec);
        }
        double t1 = now_ns();
        for (long i = 0; i < iters; i++) {
            struct xpb_decoder* dec = xpb_decoder_create(golden, glen);
            sink += decode_ops_arr(dec, ops);
            xpb_decoder_destroy(dec);
        }
        double dec_ns = (now_ns() - t1) / (double)iters;

        if (vi > 0) printf(",");
        printf("{\"name\":\"%s\",\"encodeNs\":%.3f,\"decodeNs\":%.3f,\"wireSize\":%zu}",
               name, enc_ns, dec_ns, glen);
        free(gtext);
    }
    printf("]\n");

    jv_free(manifest);
    /* Reference sink so the timed loops are not optimized away. */
    if (sink == UINT64_MAX) fprintf(stderr, "sink=%" PRIu64 "\n", sink);
    return 0;
}
