/*
 * XPB V2 C Runtime Tests
 * Tests round-trip encoding/decoding for all types
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <math.h>
#include <xpb/xpb.h>

static int tests_passed = 0;
static int tests_failed = 0;

#define TEST(name, cond) do { \
    if (cond) { \
        printf("  [PASS] %s\n", name); \
        tests_passed++; \
    } else { \
        printf("  [FAIL] %s\n", name); \
        tests_failed++; \
    } \
} while(0)

/* Test bool round-trip */
static void test_bool() {
    printf("\n=== Test Bool ===\n");

    struct xpb_encoder* enc = xpb_encoder_create(64);

    xpb_encoder_write_bool(enc, true);
    xpb_encoder_write_bool(enc, false);

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    TEST("bool true", xpb_decoder_read_bool(dec) == true);
    TEST("bool false", xpb_decoder_read_bool(dec) == false);
    xpb_decoder_destroy(dec);

    free(data);
}

/* Test int32 round-trip */
static void test_int32() {
    printf("\n=== Test Int32 ===\n");

    int32_t values[] = {0, 1, -1, 100, -100, 2147483647, -2147483648};
    int num = sizeof(values) / sizeof(values[0]);

    struct xpb_encoder* enc = xpb_encoder_create(256);
    for (int i = 0; i < num; i++) {
        xpb_encoder_write_int32(enc, values[i]);
    }

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    for (int i = 0; i < num; i++) {
        char name[64];
        snprintf(name, sizeof(name), "int32 value %d", values[i]);
        TEST(name, xpb_decoder_read_int32(dec) == values[i]);
    }
    xpb_decoder_destroy(dec);

    free(data);
}

/* Test int64 round-trip */
static void test_int64() {
    printf("\n=== Test Int64 ===\n");

    int64_t values[] = {0, 1, -1, 1000000000LL, -1000000000LL, 9223372036854775807LL, -9223372036854775807LL};
    int num = sizeof(values) / sizeof(values[0]);

    struct xpb_encoder* enc = xpb_encoder_create(256);
    for (int i = 0; i < num; i++) {
        xpb_encoder_write_int64(enc, values[i]);
    }

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    for (int i = 0; i < num; i++) {
        char name[64];
        snprintf(name, sizeof(name), "int64 value %lld", (long long)values[i]);
        TEST(name, xpb_decoder_read_int64(dec) == values[i]);
    }
    xpb_decoder_destroy(dec);

    free(data);
}

/* Test float32 round-trip */
static void test_float32() {
    printf("\n=== Test Float32 ===\n");

    float values[] = {0.0f, 1.0f, -1.0f, 3.14159f, -273.15f};
    int num = sizeof(values) / sizeof(values[0]);

    struct xpb_encoder* enc = xpb_encoder_create(256);
    for (int i = 0; i < num; i++) {
        xpb_encoder_write_float32(enc, values[i]);
    }

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    for (int i = 0; i < num; i++) {
        char name[64];
        float decoded = xpb_decoder_read_float32(dec);
        snprintf(name, sizeof(name), "float32 value %f", values[i]);
        TEST(name, fabsf(decoded - values[i]) < 0.0001f);
    }
    xpb_decoder_destroy(dec);

    free(data);
}

/* Test float64 round-trip */
static void test_float64() {
    printf("\n=== Test Float64 ===\n");

    double values[] = {0.0, 1.0, -1.0, 3.14159265358979, -273.15, 1e100};
    int num = sizeof(values) / sizeof(values[0]);

    struct xpb_encoder* enc = xpb_encoder_create(256);
    for (int i = 0; i < num; i++) {
        xpb_encoder_write_float64(enc, values[i]);
    }

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    for (int i = 0; i < num; i++) {
        char name[64];
        double decoded = xpb_decoder_read_float64(dec);
        snprintf(name, sizeof(name), "float64 value %g", values[i]);
        TEST(name, fabs(decoded - values[i]) < 1e-10);
    }
    xpb_decoder_destroy(dec);

    free(data);
}

/* Test string round-trip with various lengths */
static void test_string() {
    printf("\n=== Test String ===\n");

    const char* values[] = {"", "a", "hello", "hello world", "中文测试", "1234567890", "This is a longer string with many characters"};
    int num = sizeof(values) / sizeof(values[0]);

    for (int i = 0; i < num; i++) {
        struct xpb_encoder* enc = xpb_encoder_create(256);
        xpb_encoder_write_string(enc, values[i]);

        size_t len;
        uint8_t* data = xpb_encoder_finish(enc, &len);
        xpb_encoder_destroy(enc);

        struct xpb_decoder* dec = xpb_decoder_create(data, len);
        char* decoded = xpb_decoder_read_string(dec);

        char name[128];
        snprintf(name, sizeof(name), "string '%s'", values[i]);
        TEST(name, strcmp(decoded, values[i]) == 0);

        xpb_free(decoded);
        xpb_decoder_destroy(dec);
        free(data);
    }

    /* Test compact length threshold (254) */
    char long_str[301];
    memset(long_str, 'x', 300);
    long_str[300] = '\0';
    struct xpb_encoder* enc = xpb_encoder_create(512);
    xpb_encoder_write_string(enc, long_str);

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    char* decoded = xpb_decoder_read_string(dec);

    TEST("long string (>254 chars)", strlen(decoded) == 300);
    TEST("long string content", memcmp(decoded, long_str, 300) == 0);

    xpb_free(decoded);
    xpb_decoder_destroy(dec);
    free(data);
}

/* Test bytes round-trip */
static void test_bytes() {
    printf("\n=== Test Bytes ===\n");

    uint8_t data1[] = {0x01, 0x02, 0x03, 0x04, 0x05};
    uint8_t data2[256];
    for (int i = 0; i < 256; i++) data2[i] = (uint8_t)i;

    struct xpb_encoder* enc = xpb_encoder_create(512);
    xpb_encoder_write_bytes(enc, data1, sizeof(data1));
    xpb_encoder_write_bytes(enc, data2, sizeof(data2));

    size_t len;
    uint8_t* encoded = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(encoded, len);
    size_t out_len1;
    uint8_t* decoded1 = xpb_decoder_read_bytes(dec, &out_len1);
    size_t out_len2;
    uint8_t* decoded2 = xpb_decoder_read_bytes(dec, &out_len2);

    TEST("small bytes length", out_len1 == sizeof(data1));
    TEST("small bytes content", memcmp(decoded1, data1, sizeof(data1)) == 0);
    TEST("large bytes length", out_len2 == sizeof(data2));
    TEST("large bytes content", memcmp(decoded2, data2, sizeof(data2)) == 0);

    xpb_free(decoded1);
    xpb_free(decoded2);
    xpb_decoder_destroy(dec);
    free(encoded);
}

/* Test nested messages */
static void test_nested_message() {
    printf("\n=== Test Nested Message ===\n");

    /* Encode inner message */
    struct xpb_encoder* inner = xpb_encoder_create(64);
    xpb_encoder_write_string(inner, "inner_value");
    xpb_encoder_write_int32(inner, 42);
    size_t inner_len;
    uint8_t* inner_data = xpb_encoder_finish(inner, &inner_len);
    xpb_encoder_destroy(inner);

    /* Encode outer message with inner as bytes */
    struct xpb_encoder* outer = xpb_encoder_create(256);
    xpb_encoder_write_string(outer, "outer_name");
    xpb_encoder_write_message(outer, inner_data, inner_len);
    size_t outer_len;
    uint8_t* outer_data = xpb_encoder_finish(outer, &outer_len);
    xpb_encoder_destroy(outer);

    free(inner_data);

    /* Decode */
    struct xpb_decoder* dec = xpb_decoder_create(outer_data, outer_len);
    char* name = xpb_decoder_read_string(dec);
    size_t inner_out_len;
    uint8_t* inner_out = xpb_decoder_read_message_bytes(dec, &inner_out_len);

    TEST("outer string", strcmp(name, "outer_name") == 0);
    TEST("inner message length", inner_out_len == inner_len);

    struct xpb_decoder* inner_dec = xpb_decoder_create(inner_out, inner_out_len);
    char* inner_str = xpb_decoder_read_string(inner_dec);
    int32_t inner_int = xpb_decoder_read_int32(inner_dec);
    xpb_decoder_destroy(inner_dec);

    TEST("inner string", strcmp(inner_str, "inner_value") == 0);
    TEST("inner int", inner_int == 42);

    xpb_free(name);
    xpb_free(inner_str);
    xpb_free(inner_out);
    xpb_decoder_destroy(dec);
    free(outer_data);
}

/* Combined test with all types */
static void test_all_types() {
    printf("\n=== Test All Types Combined ===\n");

    struct xpb_encoder* enc = xpb_encoder_create(256);
    xpb_encoder_write_bool(enc, true);
    xpb_encoder_write_int32(enc, -12345);
    xpb_encoder_write_int64(enc, 9876543210LL);
    xpb_encoder_write_float32(enc, 3.14f);
    xpb_encoder_write_float64(enc, 2.718281828);
    xpb_encoder_write_string(enc, "test string");
    xpb_encoder_write_bytes(enc, (uint8_t*)"\xDE\xAD\xBE\xEF", 4);

    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    TEST("bool", xpb_decoder_read_bool(dec) == true);
    TEST("int32", xpb_decoder_read_int32(dec) == -12345);
    TEST("int64", xpb_decoder_read_int64(dec) == 9876543210LL);
    TEST("float32", fabsf(xpb_decoder_read_float32(dec) - 3.14f) < 0.001f);
    TEST("float64", fabs(xpb_decoder_read_float64(dec) - 2.718281828) < 1e-9);
    TEST("string", strcmp(xpb_decoder_read_string(dec), "test string") == 0);
    size_t bytes_len;
    uint8_t* bytes = xpb_decoder_read_bytes(dec, &bytes_len);
    TEST("bytes length", bytes_len == 4);
    TEST("bytes content", memcmp(bytes, "\xDE\xAD\xBE\xEF", 4) == 0);
    xpb_free(bytes);
    xpb_decoder_destroy(dec);

    free(data);
}

int main() {
    printf("===========================================\n");
    printf("XPB V2 C Runtime Tests\n");
    printf("===========================================\n");

    test_bool();
    test_int32();
    test_int64();
    test_float32();
    test_float64();
    test_string();
    test_bytes();
    test_nested_message();
    test_all_types();

    printf("\n===========================================\n");
    printf("Results: %d passed, %d failed\n", tests_passed, tests_failed);
    printf("===========================================\n");

    return tests_failed > 0 ? 1 : 0;
}
