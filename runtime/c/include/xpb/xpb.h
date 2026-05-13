#ifndef XPB_H
#define XPB_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Constants */
#define XPB_COMPACT_LENGTH_THRESHOLD 254
#define XPB_COMPACT_LENGTH_MARKER 0xFF

#define XPB_SIZE_BOOL 1
#define XPB_SIZE_INT32 4
#define XPB_SIZE_INT64 8
#define XPB_SIZE_UINT32 4
#define XPB_SIZE_UINT64 8
#define XPB_SIZE_FLOAT32 4
#define XPB_SIZE_FLOAT64 8

/* Cap on nested-message decode recursion. Mirrors xpb.MaxDecodeDepth in
 * the Go runtime / MaxDecodeDepth in TS. Generated T_unmarshal_at(depth)
 * shims compare against this before doing any work. */
#define XPB_MAX_DECODE_DEPTH 64

/* Forward declarations */
struct xpb_encoder;
struct xpb_decoder;

/* Encoder API.
 *
 * `xpb_encoder_create` returns NULL on allocation failure; callers MUST
 * check before using the returned encoder. Every `write_*` and `finish`
 * function additionally sets a sticky error flag on internal realloc
 * failure (mirrors the decoder's sticky-error pattern). After a sticky
 * error, every subsequent write becomes a no-op; finish() returns NULL
 * and zeroes *out_len. Use `xpb_encoder_ok()` to test before trusting
 * the encoded output. */
struct xpb_encoder* xpb_encoder_create(size_t initial_capacity);
void xpb_encoder_destroy(struct xpb_encoder* enc);

void xpb_encoder_reset(struct xpb_encoder* enc);
bool xpb_encoder_ok(const struct xpb_encoder* enc);
uint8_t* xpb_encoder_finish(struct xpb_encoder* enc, size_t* out_len);

void xpb_encoder_write_bool(struct xpb_encoder* enc, bool v);
void xpb_encoder_write_int32(struct xpb_encoder* enc, int32_t v);
void xpb_encoder_write_int64(struct xpb_encoder* enc, int64_t v);
void xpb_encoder_write_uint32(struct xpb_encoder* enc, uint32_t v);
void xpb_encoder_write_uint64(struct xpb_encoder* enc, uint64_t v);
void xpb_encoder_write_float32(struct xpb_encoder* enc, float v);
void xpb_encoder_write_float64(struct xpb_encoder* enc, double v);
void xpb_encoder_write_string(struct xpb_encoder* enc, const char* v);
void xpb_encoder_write_bytes(struct xpb_encoder* enc, const uint8_t* data, size_t len);
void xpb_encoder_write_message(struct xpb_encoder* enc, const uint8_t* data, size_t len);

/* Decoder API */
struct xpb_decoder* xpb_decoder_create(const uint8_t* data, size_t len);
void xpb_decoder_destroy(struct xpb_decoder* dec);

bool xpb_decoder_eof(struct xpb_decoder* dec);
size_t xpb_decoder_remaining(struct xpb_decoder* dec);

/*
 * Returns true if no error has been encountered while decoding. Once any
 * read overflows the buffer, encounters a malformed length, or fails an
 * allocation, the decoder enters a sticky error state and every
 * subsequent read returns a zero/NULL value with no side effects. Callers
 * MUST check xpb_decoder_ok() after a sequence of reads before trusting
 * the values, or check it after each read for fail-fast semantics.
 */
bool xpb_decoder_ok(const struct xpb_decoder* dec);

bool xpb_decoder_read_bool(struct xpb_decoder* dec);
int32_t xpb_decoder_read_int32(struct xpb_decoder* dec);
int64_t xpb_decoder_read_int64(struct xpb_decoder* dec);
uint32_t xpb_decoder_read_uint32(struct xpb_decoder* dec);
uint64_t xpb_decoder_read_uint64(struct xpb_decoder* dec);
float xpb_decoder_read_float32(struct xpb_decoder* dec);
double xpb_decoder_read_float64(struct xpb_decoder* dec);
char* xpb_decoder_read_string(struct xpb_decoder* dec);
uint8_t* xpb_decoder_read_bytes(struct xpb_decoder* dec, size_t* out_len);
uint8_t* xpb_decoder_read_message_bytes(struct xpb_decoder* dec, size_t* out_len);
void xpb_decoder_skip(struct xpb_decoder* dec, size_t n);

/* Validate and return an array length read from the wire. The caller MUST
 * pass `max_elements`; the runtime does NOT pick a default. A count that
 * is negative, exceeds `max_elements`, or cannot fit in the remaining
 * buffer at `element_min_bytes` per element is rejected before any
 * allocation. Pass element_min_bytes=0 to skip the buffer bound (only safe
 * for fully trusted input). Returns true on success with *out_count set;
 * false on failure (decoder sticky-error latched, *out_count zeroed). */
bool xpb_decoder_validate_array_count(
    struct xpb_decoder* dec,
    size_t element_min_bytes,
    size_t max_elements,
    size_t* out_count
);

/* Array API - Arrays are encoded as: count (int32) + elements */
void xpb_encoder_write_array_int32(struct xpb_encoder* enc, const int32_t* arr, size_t count);
void xpb_encoder_write_array_int64(struct xpb_encoder* enc, const int64_t* arr, size_t count);
void xpb_encoder_write_array_uint32(struct xpb_encoder* enc, const uint32_t* arr, size_t count);
void xpb_encoder_write_array_uint64(struct xpb_encoder* enc, const uint64_t* arr, size_t count);
void xpb_encoder_write_array_float32(struct xpb_encoder* enc, const float* arr, size_t count);
void xpb_encoder_write_array_float64(struct xpb_encoder* enc, const double* arr, size_t count);
void xpb_encoder_write_array_bool(struct xpb_encoder* enc, const bool* arr, size_t count);
void xpb_encoder_write_array_string(struct xpb_encoder* enc, const char** arr, size_t count);

/* Each xpb_decoder_read_array_* requires the caller to pass `max_elements`
 * explicitly. The runtime does not pick a default budget — every call
 * site declares its policy. See xpb_decoder_validate_array_count for the
 * full semantics. */
int32_t* xpb_decoder_read_array_int32(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
int64_t* xpb_decoder_read_array_int64(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
uint32_t* xpb_decoder_read_array_uint32(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
uint64_t* xpb_decoder_read_array_uint64(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
float* xpb_decoder_read_array_float32(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
double* xpb_decoder_read_array_float64(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
bool* xpb_decoder_read_array_bool(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);
char** xpb_decoder_read_array_string(struct xpb_decoder* dec, size_t max_elements, size_t* out_count);

/* Utility - free string/bytes allocated by decoder */
void xpb_free(void* ptr);
void xpb_free_array(void* ptr, size_t count, size_t elem_size);

#ifdef __cplusplus
}
#endif

#endif /* XPB_H */
