#include "xpb/xpb.h"
#include <stdlib.h>
#include <string.h>

/* Encoder implementation */
struct xpb_encoder {
    uint8_t* buf;
    size_t capacity;
    size_t pos;
};

static void xpb_encoder_ensure_capacity(struct xpb_encoder* enc, size_t needed) {
    if (enc->pos + needed > enc->capacity) {
        size_t new_capacity = enc->capacity * 2;
        if (new_capacity < enc->pos + needed) {
            new_capacity = enc->pos + needed;
        }
        enc->buf = (uint8_t*)realloc(enc->buf, new_capacity);
        enc->capacity = new_capacity;
    }
}

struct xpb_encoder* xpb_encoder_create(size_t initial_capacity) {
    struct xpb_encoder* enc = (struct xpb_encoder*)malloc(sizeof(struct xpb_encoder));
    enc->buf = (uint8_t*)malloc(initial_capacity);
    enc->capacity = initial_capacity;
    enc->pos = 0;
    return enc;
}

void xpb_encoder_destroy(struct xpb_encoder* enc) {
    if (enc) {
        free(enc->buf);
        free(enc);
    }
}

void xpb_encoder_reset(struct xpb_encoder* enc) {
    enc->pos = 0;
}

uint8_t* xpb_encoder_finish(struct xpb_encoder* enc, size_t* out_len) {
    uint8_t* result = (uint8_t*)malloc(enc->pos);
    memcpy(result, enc->buf, enc->pos);
    if (out_len) *out_len = enc->pos;
    return result;
}

static void xpb_encoder_write_le32(struct xpb_encoder* enc, uint32_t v) {
#if defined(_WIN32) || defined(__LITTLE_ENDIAN__)
    enc->buf[enc->pos++] = (uint8_t)(v & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 8) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 16) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 24) & 0xFF);
#else
    uint32_t swapped = __builtin_bswap32(v);
    memcpy(&enc->buf[enc->pos], &swapped, 4);
    enc->pos += 4;
#endif
}

static void xpb_encoder_write_le64(struct xpb_encoder* enc, uint64_t v) {
#if defined(_WIN32) || defined(__LITTLE_ENDIAN__)
    enc->buf[enc->pos++] = (uint8_t)(v & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 8) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 16) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 24) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 32) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 40) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 48) & 0xFF);
    enc->buf[enc->pos++] = (uint8_t)((v >> 56) & 0xFF);
#else
    uint64_t swapped = __builtin_bswap64(v);
    memcpy(&enc->buf[enc->pos], &swapped, 8);
    enc->pos += 8;
#endif
}

void xpb_encoder_write_bool(struct xpb_encoder* enc, bool v) {
    xpb_encoder_ensure_capacity(enc, 1);
    enc->buf[enc->pos++] = v ? 1 : 0;
}

void xpb_encoder_write_int32(struct xpb_encoder* enc, int32_t v) {
    xpb_encoder_ensure_capacity(enc, 4);
    xpb_encoder_write_le32(enc, (uint32_t)v);
}

void xpb_encoder_write_int64(struct xpb_encoder* enc, int64_t v) {
    xpb_encoder_ensure_capacity(enc, 8);
    xpb_encoder_write_le64(enc, (uint64_t)v);
}

void xpb_encoder_write_uint32(struct xpb_encoder* enc, uint32_t v) {
    xpb_encoder_ensure_capacity(enc, 4);
    xpb_encoder_write_le32(enc, v);
}

void xpb_encoder_write_uint64(struct xpb_encoder* enc, uint64_t v) {
    xpb_encoder_ensure_capacity(enc, 8);
    xpb_encoder_write_le64(enc, v);
}

void xpb_encoder_write_float32(struct xpb_encoder* enc, float v) {
    uint32_t bits;
    memcpy(&bits, &v, sizeof(bits));
    xpb_encoder_write_uint32(enc, bits);
}

void xpb_encoder_write_float64(struct xpb_encoder* enc, double v) {
    uint64_t bits;
    memcpy(&bits, &v, sizeof(bits));
    xpb_encoder_write_uint64(enc, bits);
}

static void xpb_encoder_write_compact_length(struct xpb_encoder* enc, size_t len) {
    if (len <= XPB_COMPACT_LENGTH_THRESHOLD) {
        xpb_encoder_ensure_capacity(enc, 1);
        enc->buf[enc->pos++] = (uint8_t)len;
    } else {
        xpb_encoder_ensure_capacity(enc, 5);
        enc->buf[enc->pos++] = XPB_COMPACT_LENGTH_MARKER;
        xpb_encoder_write_le32(enc, (uint32_t)len);
    }
}

void xpb_encoder_write_string(struct xpb_encoder* enc, const char* v) {
    size_t len = strlen(v);
    xpb_encoder_write_compact_length(enc, len);
    xpb_encoder_ensure_capacity(enc, len);
    memcpy(&enc->buf[enc->pos], v, len);
    enc->pos += len;
}

void xpb_encoder_write_bytes(struct xpb_encoder* enc, const uint8_t* data, size_t len) {
    xpb_encoder_write_compact_length(enc, len);
    xpb_encoder_ensure_capacity(enc, len);
    memcpy(&enc->buf[enc->pos], data, len);
    enc->pos += len;
}

void xpb_encoder_write_message(struct xpb_encoder* enc, const uint8_t* data, size_t len) {
    xpb_encoder_write_bytes(enc, data, len);
}

/* Decoder implementation */
struct xpb_decoder {
    const uint8_t* data;
    size_t len;
    size_t pos;
    bool error; /* sticky: once set, every read becomes a no-op */
};

struct xpb_decoder* xpb_decoder_create(const uint8_t* data, size_t len) {
    struct xpb_decoder* dec = (struct xpb_decoder*)malloc(sizeof(struct xpb_decoder));
    if (dec == NULL) return NULL;
    dec->data = data;
    dec->len = len;
    dec->pos = 0;
    dec->error = false;
    return dec;
}

bool xpb_decoder_ok(const struct xpb_decoder* dec) {
    return dec != NULL && !dec->error;
}

/*
 * xpb_decoder_can_read: returns true if n more bytes are available, sets
 * the sticky error flag and returns false otherwise. Every read function
 * funnels through this so a single malformed length can't run off the end
 * of the buffer.
 */
static bool xpb_decoder_can_read(struct xpb_decoder* dec, size_t n) {
    if (dec->error) return false;
    if (n > dec->len || dec->pos > dec->len - n) {
        dec->error = true;
        return false;
    }
    return true;
}

void xpb_decoder_destroy(struct xpb_decoder* dec) {
    free(dec);
}

bool xpb_decoder_eof(struct xpb_decoder* dec) {
    return dec->pos >= dec->len;
}

size_t xpb_decoder_remaining(struct xpb_decoder* dec) {
    return dec->len - dec->pos;
}

/*
 * xpb_decoder_read_le32: caller must hold the bounds check before this is
 * invoked. We keep this pattern because it's called from int32 / uint32 /
 * float32 / compact-length paths — each does its own xpb_decoder_can_read.
 */
static uint32_t xpb_decoder_read_le32(struct xpb_decoder* dec) {
    uint32_t v;
#if defined(_WIN32) || defined(__LITTLE_ENDIAN__)
    v = dec->data[dec->pos] |
        (dec->data[dec->pos + 1] << 8) |
        (dec->data[dec->pos + 2] << 16) |
        (dec->data[dec->pos + 3] << 24);
#else
    memcpy(&v, &dec->data[dec->pos], 4);
    v = __builtin_bswap32(v);
#endif
    dec->pos += 4;
    return v;
}

static uint64_t xpb_decoder_read_le64(struct xpb_decoder* dec) {
    uint64_t v;
#if defined(_WIN32) || defined(__LITTLE_ENDIAN__)
    uint32_t lo = dec->data[dec->pos] |
                  (dec->data[dec->pos + 1] << 8) |
                  (dec->data[dec->pos + 2] << 16) |
                  (dec->data[dec->pos + 3] << 24);
    uint32_t hi = dec->data[dec->pos + 4] |
                  (dec->data[dec->pos + 5] << 8) |
                  (dec->data[dec->pos + 6] << 16) |
                  (dec->data[dec->pos + 7] << 24);
    v = ((uint64_t)lo) | ((uint64_t)hi << 32);
#else
    memcpy(&v, &dec->data[dec->pos], 8);
    v = __builtin_bswap64(v);
#endif
    dec->pos += 8;
    return v;
}

bool xpb_decoder_read_bool(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 1)) return false;
    return dec->data[dec->pos++] != 0;
}

int32_t xpb_decoder_read_int32(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 4)) return 0;
    return (int32_t)xpb_decoder_read_le32(dec);
}

int64_t xpb_decoder_read_int64(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 8)) return 0;
    return (int64_t)xpb_decoder_read_le64(dec);
}

uint32_t xpb_decoder_read_uint32(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 4)) return 0;
    return xpb_decoder_read_le32(dec);
}

uint64_t xpb_decoder_read_uint64(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 8)) return 0;
    return xpb_decoder_read_le64(dec);
}

float xpb_decoder_read_float32(struct xpb_decoder* dec) {
    uint32_t bits = xpb_decoder_read_uint32(dec);
    float v;
    memcpy(&v, &bits, sizeof(v));
    return v;
}

double xpb_decoder_read_float64(struct xpb_decoder* dec) {
    uint64_t bits = xpb_decoder_read_uint64(dec);
    double v;
    memcpy(&v, &bits, sizeof(v));
    return v;
}

static size_t xpb_decoder_read_compact_length(struct xpb_decoder* dec) {
    if (!xpb_decoder_can_read(dec, 1)) return 0;
    uint8_t first = dec->data[dec->pos++];
    if (first != XPB_COMPACT_LENGTH_MARKER) {
        return first;
    }
    if (!xpb_decoder_can_read(dec, 4)) return 0;
    return xpb_decoder_read_le32(dec);
}

char* xpb_decoder_read_string(struct xpb_decoder* dec) {
    size_t len = xpb_decoder_read_compact_length(dec);
    if (!xpb_decoder_can_read(dec, len)) return NULL;
    char* v = (char*)malloc(len + 1);
    if (v == NULL) {
        dec->error = true;
        return NULL;
    }
    memcpy(v, &dec->data[dec->pos], len);
    v[len] = '\0';
    dec->pos += len;
    return v;
}

uint8_t* xpb_decoder_read_bytes(struct xpb_decoder* dec, size_t* out_len) {
    size_t len = xpb_decoder_read_compact_length(dec);
    if (!xpb_decoder_can_read(dec, len)) {
        if (out_len) *out_len = 0;
        return NULL;
    }
    /* malloc(0) is implementation-defined; return NULL with len=0. */
    if (len == 0) {
        if (out_len) *out_len = 0;
        return NULL;
    }
    uint8_t* v = (uint8_t*)malloc(len);
    if (v == NULL) {
        dec->error = true;
        if (out_len) *out_len = 0;
        return NULL;
    }
    memcpy(v, &dec->data[dec->pos], len);
    dec->pos += len;
    if (out_len) *out_len = len;
    return v;
}

uint8_t* xpb_decoder_read_message_bytes(struct xpb_decoder* dec, size_t* out_len) {
    return xpb_decoder_read_bytes(dec, out_len);
}

void xpb_decoder_skip(struct xpb_decoder* dec, size_t n) {
    if (!xpb_decoder_can_read(dec, n)) return;
    dec->pos += n;
}

void xpb_free(void* ptr) {
    free(ptr);
}

void xpb_free_array(void* ptr, size_t count, size_t elem_size) {
    (void)count;
    (void)elem_size;
    free(ptr);
}

/* Array encoding implementations */
void xpb_encoder_write_array_int32(struct xpb_encoder* enc, const int32_t* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_int32(enc, arr[i]);
    }
}

void xpb_encoder_write_array_int64(struct xpb_encoder* enc, const int64_t* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_int64(enc, arr[i]);
    }
}

void xpb_encoder_write_array_uint32(struct xpb_encoder* enc, const uint32_t* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_uint32(enc, arr[i]);
    }
}

void xpb_encoder_write_array_uint64(struct xpb_encoder* enc, const uint64_t* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_uint64(enc, arr[i]);
    }
}

void xpb_encoder_write_array_float32(struct xpb_encoder* enc, const float* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_float32(enc, arr[i]);
    }
}

void xpb_encoder_write_array_float64(struct xpb_encoder* enc, const double* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_float64(enc, arr[i]);
    }
}

void xpb_encoder_write_array_bool(struct xpb_encoder* enc, const bool* arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_bool(enc, arr[i]);
    }
}

void xpb_encoder_write_array_string(struct xpb_encoder* enc, const char** arr, size_t count) {
    xpb_encoder_write_int32(enc, (int32_t)count);
    for (size_t i = 0; i < count; i++) {
        xpb_encoder_write_string(enc, arr[i]);
    }
}

/*
 * xpb_decoder_validate_array_count: read a 4-byte count and validate it
 * against the per-element minimum on-wire size (each element occupies at
 * least element_min_bytes). Rejects negative counts and counts that
 * cannot possibly fit in the remaining buffer. Returns true on success
 * with *out_count set; false on failure (sticky decoder error set, *out_count = 0).
 */
static bool xpb_decoder_validate_array_count(
    struct xpb_decoder* dec, size_t element_min_bytes, size_t* out_count
) {
    int32_t count = xpb_decoder_read_int32(dec);
    if (dec->error || count < 0) {
        dec->error = true;
        if (out_count) *out_count = 0;
        return false;
    }
    if (element_min_bytes > 0) {
        size_t remaining = dec->len - dec->pos;
        size_t max = remaining / element_min_bytes;
        if ((size_t)count > max) {
            dec->error = true;
            if (out_count) *out_count = 0;
            return false;
        }
    }
    if (out_count) *out_count = (size_t)count;
    return true;
}

/* Array decoding implementations */
int32_t* xpb_decoder_read_array_int32(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(int32_t), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    int32_t* arr = (int32_t*)malloc(count * sizeof(int32_t));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_int32(dec);
    }
    return arr;
}

int64_t* xpb_decoder_read_array_int64(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(int64_t), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    int64_t* arr = (int64_t*)malloc(count * sizeof(int64_t));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_int64(dec);
    }
    return arr;
}

uint32_t* xpb_decoder_read_array_uint32(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(uint32_t), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    uint32_t* arr = (uint32_t*)malloc(count * sizeof(uint32_t));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_uint32(dec);
    }
    return arr;
}

uint64_t* xpb_decoder_read_array_uint64(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(uint64_t), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    uint64_t* arr = (uint64_t*)malloc(count * sizeof(uint64_t));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_uint64(dec);
    }
    return arr;
}

float* xpb_decoder_read_array_float32(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(float), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    float* arr = (float*)malloc(count * sizeof(float));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_float32(dec);
    }
    return arr;
}

double* xpb_decoder_read_array_float64(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(double), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    double* arr = (double*)malloc(count * sizeof(double));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_float64(dec);
    }
    return arr;
}

bool* xpb_decoder_read_array_bool(struct xpb_decoder* dec, size_t* out_count) {
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, sizeof(bool), &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    bool* arr = (bool*)malloc(count * sizeof(bool));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_bool(dec);
    }
    return arr;
}

char** xpb_decoder_read_array_string(struct xpb_decoder* dec, size_t* out_count) {
    /* Strings are variable-length; minimum on-wire size per element is 1
     * byte (the compact-length prefix for an empty string). */
    size_t count = 0;
    if (!xpb_decoder_validate_array_count(dec, 1, &count)) {
        if (out_count) *out_count = 0;
        return NULL;
    }
    if (out_count) *out_count = count;
    if (count == 0) return NULL;
    char** arr = (char**)malloc(count * sizeof(char*));
    if (arr == NULL) { dec->error = true; if (out_count) *out_count = 0; return NULL; }
    for (size_t i = 0; i < count; i++) {
        arr[i] = xpb_decoder_read_string(dec);
    }
    return arr;
}
