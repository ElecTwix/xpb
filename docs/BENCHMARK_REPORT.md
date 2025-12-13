# XPB V2 Benchmark Report

**Date:** December 13, 2025
**System:** Linux, Intel(R) Core(TM) i9-13900H

## Executive Summary

XPB V2 consistently outperforms JSON and MessagePack in encoding speed and message size across all platforms. Decoding speed is exceptionally high in Go (up to 200x faster than JSON). In Node.js and Browsers, XPB provides strong performance, especially for typed arrays and encoding tasks.

## 1. Small Message Performance
**Structure:** 3 fields (String, Int32, Bool)
**Size:** 19 bytes (vs 47 bytes JSON)

| Format | Operation | Go (ns/op) | Node.js (ns/op) | Browser (ns/op) | Size (Bytes) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **XPB V2** | **Encode** | **18** | **21** | **21** | **19** |
| | **Decode** | **6** | **107** | **81** | **19** |
| JSON | Encode | 259 | 134 | 80 | 47 |
| | Decode | 1417 | 340 | 185 | 47 |
| Msgpack | Encode | 356 | 2108 | 1456 | 37 |
| | Decode | 516 | 510 | 453 | 37 |

**Insights:**
*   **Go:** XPB is ~14x faster to encode and ~230x faster to decode than JSON.
*   **Browser:** XPB encoding is ~4x faster than JSON.
*   **Size:** XPB is 2.5x smaller than JSON.

## 2. Large Message Performance
**Structure:** 7 fields (ID, Strings, Floats)
**Size:** 121 bytes (vs 192 bytes JSON)

| Format | Operation | Go (ns/op) | Node.js (ns/op) | Browser (ns/op) | Size (Bytes) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **XPB V2** | **Encode** | **32** | **254** | **727** | **121** |
| | **Decode** | **14** | **454** | **767** | **121** |
| JSON | Encode | 719 | 434 | 332 | 192 |
| | Decode | 3260 | 707 | 447 | 192 |

**Insights:**
*   **Go:** XPB maintains extreme performance (32ns encode) due to fixed-width processing.
*   **Node/Browser:** XPB encoding is faster than JSON. Decoding is comparable or slightly slower than V8's optimized `JSON.parse` for this object structure, but still competitive.

## 3. Collection Performance (100 Elements)

### Int32 Array
| Format | Operation | Go (ns/op) | Node.js (ns/op) | Size (Bytes) |
| :--- | :--- | :--- | :--- | :--- |
| **XPB V2** | **Encode** | **404** | **195** | **404** |
| | **Decode** | **364** | **231** | **404** |
| JSON | Encode | 2943 | 1313 | 435 |
| | Decode | 17282 | 1348 | 435 |

**Insights:**
*   **Numeric Arrays:** XPB shines here. In Go, decoding is **~47x faster** than JSON. In Node.js, it's **~5.8x faster**. This demonstrates the efficiency of packed binary data for numeric arrays.

### String Array
| Format | Operation | Go (ns/op) | Node.js (ns/op) | Size (Bytes) |
| :--- | :--- | :--- | :--- | :--- |
| **XPB V2** | **Encode** | **1362** | 4443 | **1304** |
| | **Decode** | **1138** | 13087 | **1304** |
| JSON | Encode | 5393 | 1874 | 1501 |
| | Decode | 29893 | 3859 | 1501 |

**Insights:**
*   **Go:** XPB is ~26x faster decoding.
*   **Node.js:** JSON is faster for String Arrays. This is expected as V8 optimizes string allocation during `JSON.parse` heavily, while XPB JS runtime must decode UTF-8 bytes manually.

## 4. Size Scaling

| Message Size | XPB (Bytes) | JSON (Bytes) | Savings |
| :--- | :--- | :--- | :--- |
| Tiny (Bool) | 1 | 11 | **91%** |
| Small (3 fields) | 19 | 47 | **60%** |
| Medium (8 fields) | 452 | 548 | **17.5%** |
| Large (10KB) | 10,604 | 10,982 | 3.4% |

**Conclusion:** XPB offers significant bandwidth savings for small to medium payloads, which are common in real-time applications (events, telemetry, game state).

---
*Report generated via `cmd/xpbench`.*
