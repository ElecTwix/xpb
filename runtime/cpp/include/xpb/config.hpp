#ifndef XPB_CONFIG_HPP
#define XPB_CONFIG_HPP

#include <cstdint>
#include <cstddef>

namespace xpb {

constexpr uint8_t COMPACT_LENGTH_THRESHOLD = 254;
constexpr uint8_t COMPACT_LENGTH_MARKER = 0xFF;

constexpr size_t SIZE_BOOL = 1;
constexpr size_t SIZE_INT32 = 4;
constexpr size_t SIZE_INT64 = 8;
constexpr size_t SIZE_UINT32 = 4;
constexpr size_t SIZE_UINT64 = 8;
constexpr size_t SIZE_FLOAT32 = 4;
constexpr size_t SIZE_FLOAT64 = 8;

} // namespace xpb

#endif // XPB_CONFIG_HPP
