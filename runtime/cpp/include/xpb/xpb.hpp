#ifndef XPB_HPP
#define XPB_HPP

#include "config.hpp"
#include <vector>
#include <string>
#include <string_view>
#include <cstring>
#include <stdexcept>

#if defined(_WIN32) || defined(_WIN64)
#define XPB_LITTLE_ENDIAN 1
#elif defined(__BYTE_ORDER__)
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
#define XPB_LITTLE_ENDIAN 1
#else
#define XPB_LITTLE_ENDIAN 0
#endif
#elif defined(__LITTLE_ENDIAN__) || defined(__ARMEL__) || defined(__THUMBEL__) || defined(__AARCH64EL__)
#define XPB_LITTLE_ENDIAN 1
#elif defined(__BIG_ENDIAN__) || defined(__ARMEB__) || defined(__THUMBEB__) || defined(__AARCH64EB__)
#define XPB_LITTLE_ENDIAN 0
#else
#define XPB_LITTLE_ENDIAN 1
#endif

#if XPB_LITTLE_ENDIAN
static inline uint32_t xpb_bswap32(uint32_t v) { return v; }
static inline uint64_t xpb_bswap64(uint64_t v) { return v; }
#elif defined(_MSC_VER)
static inline uint32_t xpb_bswap32(uint32_t v) { return _byteswap_ulong(v); }
static inline uint64_t xpb_bswap64(uint64_t v) { return _byteswap_uint64(v); }
#elif defined(__GNUC__) || defined(__clang__)
static inline uint32_t xpb_bswap32(uint32_t v) { return __builtin_bswap32(v); }
static inline uint64_t xpb_bswap64(uint64_t v) { return __builtin_bswap64(v); }
#else
static inline uint32_t xpb_bswap32(uint32_t v) {
    return ((v & 0xFF) << 24) | ((v & 0xFF00) << 8) | ((v & 0xFF0000) >> 8) | ((v & 0xFF000000) >> 24);
}
static inline uint64_t xpb_bswap64(uint64_t v) {
    return ((v & 0xFFULL) << 56) | ((v & 0xFF00ULL) << 40) | ((v & 0xFF0000ULL) << 24) |
           ((v & 0xFF000000ULL) << 8) | ((v & 0xFF0000000ULL) >> 8) | ((v & 0xFF00000000ULL) >> 24) |
           ((v & 0xFF000000000ULL) >> 40) | ((v & 0xFF0000000000ULL) >> 56);
}
#endif

namespace xpb {

inline void writeLe32(uint8_t* out, uint32_t v) {
    uint32_t swapped = xpb_bswap32(v);
    std::memcpy(out, &swapped, 4);
}

inline void writeLe64(uint8_t* out, uint64_t v) {
    uint64_t swapped = xpb_bswap64(v);
    std::memcpy(out, &swapped, 8);
}

inline uint32_t readLe32(const uint8_t* in) {
    uint32_t v;
    std::memcpy(&v, in, 4);
    return xpb_bswap32(v);
}

inline uint64_t readLe64(const uint8_t* in) {
    uint64_t v;
    std::memcpy(&v, in, 8);
    return xpb_bswap64(v);
}

class Encoder {
public:
    explicit Encoder(size_t initial_capacity = 256) {
        buf_.reserve(initial_capacity);
    }

    void reset() {
        buf_.clear();
    }

    const std::vector<uint8_t>& finish() const {
        return buf_;
    }

    std::vector<uint8_t> release() {
        return std::move(buf_);
    }

    void writeBool(bool v) {
        buf_.push_back(v ? 1 : 0);
    }

    void writeInt32(int32_t v) {
        uint8_t bytes[4];
        writeLe32(bytes, static_cast<uint32_t>(v));
        buf_.insert(buf_.end(), bytes, bytes + 4);
    }

    void writeInt64(int64_t v) {
        uint8_t bytes[8];
        writeLe64(bytes, static_cast<uint64_t>(v));
        buf_.insert(buf_.end(), bytes, bytes + 8);
    }

    void writeUint32(uint32_t v) {
        uint8_t bytes[4];
        writeLe32(bytes, v);
        buf_.insert(buf_.end(), bytes, bytes + 4);
    }

    void writeUint64(uint64_t v) {
        uint8_t bytes[8];
        writeLe64(bytes, v);
        buf_.insert(buf_.end(), bytes, bytes + 8);
    }

    void writeFloat32(float v) {
        uint32_t bits;
        std::memcpy(&bits, &v, 4);
        writeUint32(bits);
    }

    void writeFloat64(double v) {
        uint64_t bits;
        std::memcpy(&bits, &v, 8);
        writeUint64(bits);
    }

    void writeString(std::string_view v) {
        size_t len = v.size();
        writeCompactLength(len);
        buf_.insert(buf_.end(), v.data(), v.data() + len);
    }

    void writeBytes(const uint8_t* data, size_t len) {
        writeCompactLength(len);
        buf_.insert(buf_.end(), data, data + len);
    }

    void writeBytes(const std::vector<uint8_t>& v) {
        writeBytes(v.data(), v.size());
    }

    void writeMessage(const std::vector<uint8_t>& data) {
        writeBytes(data);
    }

private:
    std::vector<uint8_t> buf_;

    void writeCompactLength(size_t len) {
        if (len <= COMPACT_LENGTH_THRESHOLD) {
            buf_.push_back(static_cast<uint8_t>(len));
        } else {
            buf_.push_back(COMPACT_LENGTH_MARKER);
            uint8_t bytes[4];
            writeLe32(bytes, static_cast<uint32_t>(len));
            buf_.insert(buf_.end(), bytes, bytes + 4);
        }
    }
};

class Decoder {
public:
    explicit Decoder(const uint8_t* data, size_t len) : data_(data), len_(len) {}
    explicit Decoder(const std::vector<uint8_t>& data) : data_(data.data()), len_(data.size()) {}

    bool eof() const { return pos_ >= len_; }
    size_t remaining() const { return len_ - pos_; }

    bool readBool() {
        if (pos_ >= len_) throw std::runtime_error("xpb: unexpected EOF reading bool");
        return data_[pos_++] != 0;
    }

    int32_t readInt32() {
        if (pos_ + 4 > len_) throw std::runtime_error("xpb: unexpected EOF reading int32");
        int32_t v = static_cast<int32_t>(readLe32(data_ + pos_));
        pos_ += 4;
        return v;
    }

    int64_t readInt64() {
        if (pos_ + 8 > len_) throw std::runtime_error("xpb: unexpected EOF reading int64");
        int64_t v = static_cast<int64_t>(readLe64(data_ + pos_));
        pos_ += 8;
        return v;
    }

    uint32_t readUint32() {
        if (pos_ + 4 > len_) throw std::runtime_error("xpb: unexpected EOF reading uint32");
        uint32_t v = readLe32(data_ + pos_);
        pos_ += 4;
        return v;
    }

    uint64_t readUint64() {
        if (pos_ + 8 > len_) throw std::runtime_error("xpb: unexpected EOF reading uint64");
        uint64_t v = readLe64(data_ + pos_);
        pos_ += 8;
        return v;
    }

    float readFloat32() {
        uint32_t bits = readUint32();
        float v;
        std::memcpy(&v, &bits, 4);
        return v;
    }

    double readFloat64() {
        uint64_t bits = readUint64();
        double v;
        std::memcpy(&v, &bits, 8);
        return v;
    }

    std::string readString() {
        size_t len = readCompactLength();
        if (pos_ + len > len_) throw std::runtime_error("xpb: unexpected EOF reading string");
        std::string v(reinterpret_cast<const char*>(data_ + pos_), len);
        pos_ += len;
        return v;
    }

    std::vector<uint8_t> readBytes() {
        size_t len = readCompactLength();
        if (pos_ + len > len_) throw std::runtime_error("xpb: unexpected EOF reading bytes");
        std::vector<uint8_t> v(data_ + pos_, data_ + pos_ + len);
        pos_ += len;
        return v;
    }

    std::vector<uint8_t> readMessageBytes() {
        return readBytes();
    }

    void skip(size_t n) {
        if (pos_ + n > len_) throw std::runtime_error("xpb: unexpected EOF during skip");
        pos_ += n;
    }

private:
    const uint8_t* data_;
    size_t len_;
    size_t pos_ = 0;

    size_t readCompactLength() {
        if (pos_ >= len_) throw std::runtime_error("xpb: unexpected EOF reading length");
        uint8_t first = data_[pos_++];
        if (first != COMPACT_LENGTH_MARKER) {
            return first;
        }
        if (pos_ + 4 > len_) throw std::runtime_error("xpb: unexpected EOF reading extended length");
        uint32_t len = readLe32(data_ + pos_);
        pos_ += 4;
        return len;
    }
};

} // namespace xpb

#endif // XPB_HPP
