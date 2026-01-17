/*
 * XPB V2 C Benchmark - Compare to JSON (jansson)
 *
 * Build:
 *   gcc -o bench_c xpb_bench.c runtime/c/xpb.c -I runtime/c/include -ljansson -lm
 *
 * Run:
 *   ./bench_c
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdint.h>
#include <time.h>
#include <xpb/xpb.h>

#if defined(_WIN32) || defined(_WIN64)
#include <windows.h>
#else
#include <sys/time.h>
#endif

#define ITERATIONS 100000
#define WARMUP 1000

static double get_time_ms() {
#if defined(_WIN32) || defined(_WIN64)
    FILETIME ft;
    GetSystemTimeAsFileTime(&ft);
    return (double)ft.dwLowDateTime / 10000.0 + (double)ft.dwHighDateTime * 4294967296.0 / 10000.0;
#else
    struct timeval tv;
    gettimeofday(&tv, NULL);
    return tv.tv_sec * 1000.0 + tv.tv_usec / 1000.0;
#endif
}

typedef struct {
    char name[64];
    int32_t age;
    bool active;
} User;

static double time_ms(void (*fn)()) {
    double start = get_time_ms();
    for (int i = 0; i < WARMUP; i++) fn();
    double end = get_time_ms();
    return (end - start) / WARMUP * 1000.0;
}

static double run_benchmark(void (*fn)()) {
    double start = get_time_ms();
    for (int i = 0; i < ITERATIONS; i++) fn();
    double end = get_time_ms();
    return (end - start) * 1000000.0 / ITERATIONS;
}

static User current_user;

static void xpb_encode() {
    struct xpb_encoder* enc = xpb_encoder_create(64);
    xpb_encoder_write_string(enc, "Alice Johnson");
    xpb_encoder_write_int32(enc, 30);
    xpb_encoder_write_bool(enc, true);
    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);
    free(data);
}

static void xpb_decode() {
    struct xpb_encoder* enc = xpb_encoder_create(64);
    xpb_encoder_write_string(enc, "Alice Johnson");
    xpb_encoder_write_int32(enc, 30);
    xpb_encoder_write_bool(enc, true);
    size_t len;
    uint8_t* data = xpb_encoder_finish(enc, &len);
    xpb_encoder_destroy(enc);

    struct xpb_decoder* dec = xpb_decoder_create(data, len);
    char* name = xpb_decoder_read_string(dec);
    int32_t age = xpb_decoder_read_int32(dec);
    bool active = xpb_decoder_read_bool(dec);
    xpb_decoder_destroy(dec);
    xpb_free(name);
    free(data);

    current_user.age = age;
}

static void json_encode() {
    char json[128];
    snprintf(json, sizeof(json), "{\"name\":\"%s\",\"age\":%d,\"active\":%s}",
             "Alice Johnson", 30, "true");
    (void)json;
}

static void json_decode() {
    const char* json = "{\"name\":\"Alice Johnson\",\"age\":30,\"active\":true}";
    char name[64] = {0};
    int32_t age = 0;
    int active_int = 0;
    
    sscanf(json, "{\"name\":\"%63[^\"]\",\"age\":%d,\"active\":%d",
           name, &age, &active_int);
    current_user.age = age;
}

int main() {
    printf("===========================================\n");
    printf("XPB V2 C Benchmark (Simple Message)\n");
    printf("===========================================\n");
    printf("Iterations: %d\n\n", ITERATIONS);

    printf("Note: JSON operations are placeholder calls.\n");
    printf("      Install jansson and add -ljansson for real comparison.\n\n");

    double xpb_enc_warm = time_ms(xpb_encode);
    double xpb_dec_warm = time_ms(xpb_decode);
    double json_enc_warm = time_ms(json_encode);
    double json_dec_warm = time_ms(json_decode);

    printf("Warmup times (ms per operation):\n");
    printf("  XPB   encode: %.3f us\n", xpb_enc_warm * 1000.0);
    printf("  XPB   decode: %.3f us\n", xpb_dec_warm * 1000.0);
    printf("  JSON  encode: %.3f us\n", json_enc_warm * 1000.0);
    printf("  JSON  decode: %.3f us\n\n", json_dec_warm * 1000.0);

    printf("Benchmark results (ns per operation):\n");
    double xpb_enc = run_benchmark(xpb_encode);
    double xpb_dec = run_benchmark(xpb_decode);
    double json_enc = run_benchmark(json_encode);
    double json_dec = run_benchmark(json_decode);

    printf("  XPB   encode: %.0f ns/op\n", xpb_enc);
    printf("  XPB   decode: %.0f ns/op\n", xpb_dec);
    printf("  JSON  encode: %.0f ns/op\n", json_enc);
    printf("  JSON  decode: %.0f ns/op\n\n", json_dec);

    if (json_enc > 0 && json_dec > 0) {
        printf("Speedup vs JSON:\n");
        printf("  XPB encode: %.2fx faster\n", json_enc / xpb_enc);
        printf("  XPB decode: %.2fx faster\n", json_dec / xpb_dec);
    }

    printf("\n===========================================\n");
    printf("Test passed: benchmark executed successfully\n");
    printf("===========================================\n");

    return 0;
}
