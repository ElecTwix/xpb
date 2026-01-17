-- XPB V2 Lua Runtime (Optimized)
-- Pure Lua implementation of XPB V2 binary serialization

local xpb = {}

-- Constants
xpb.COMPACT_LENGTH_THRESHOLD = 254
xpb.COMPACT_LENGTH_MARKER = 0xFF

-- Utility functions
local function le32_to_num(bytes)
    if #bytes < 4 then return 0 end
    local b1, b2, b3, b4 = bytes:byte(1, 4)
    local n = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
    if n >= 2147483648 then n = n - 4294967296 end
    return n
end

local function le64_to_num(bytes)
    if #bytes < 8 then return 0 end
    local b1, b2, b3, b4, b5, b6, b7, b8 = bytes:byte(1, 8)
    local lo = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
    local hi = b5 + (b6 << 8) + (b7 << 16) + (b8 << 24)
    local n = lo + (hi << 32)
    if n >= 9223372036854775808 then n = n - 18446744073709551616 end
    return n
end

local function num_to_le32(n)
    n = math.floor(n) % 4294967296
    return string.char(
        n & 0xFF,
        (n >> 8) & 0xFF,
        (n >> 16) & 0xFF,
        (n >> 24) & 0xFF
    )
end

local function num_to_le64(n)
    n = math.floor(n)
    local lo = n & 0xFFFFFFFF
    local hi = (n >> 32) & 0xFFFFFFFF
    return string.char(
        lo & 0xFF,
        (lo >> 8) & 0xFF,
        (lo >> 16) & 0xFF,
        (lo >> 24) & 0xFF,
        hi & 0xFF,
        (hi >> 8) & 0xFF,
        (hi >> 16) & 0xFF,
        (hi >> 24) & 0xFF
    )
end

-- Encoder (chunk-based with table buffer)
function xpb.Encoder(initial_size)
    local self = {
        chunks = {},
        pos = 0,
        chunk_size = 256
    }
    if initial_size and initial_size > 0 then
        self.chunk_size = math.max(256, math.floor(initial_size / 2))
    end
    self.chunks[1] = string.rep("\0", self.chunk_size)

    function self:ensure_capacity(needed)
        local current = self.chunks[#self.chunks]
        local used = #current
        if self.pos + needed <= used then return end
        self.chunks[#self.chunks] = current:sub(1, self.pos)
        self.chunks[#self.chunks + 1] = string.rep("\0", self.chunk_size)
        self.pos = 0
    end

    function self:write_bool(v)
        self:ensure_capacity(1)
        local current = self.chunks[#self.chunks]
        self.pos = self.pos + 1
        self.chunks[#self.chunks] = current:sub(1, self.pos - 1) .. (v and "\1" or "\0")
    end

    function self:write_int32(v)
        self:ensure_capacity(4)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        self.chunks[#self.chunks] = current:sub(1, p) ..
            string.char(v & 0xFF, (v >> 8) & 0xFF, (v >> 16) & 0xFF, (v >> 24) & 0xFF)
        self.pos = p + 4
    end

    function self:write_int64(v)
        self:ensure_capacity(8)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        local lo = v & 0xFFFFFFFF
        local hi = (v >> 32) & 0xFFFFFFFF
        self.chunks[#self.chunks] = current:sub(1, p) ..
            string.char(lo & 0xFF, (lo >> 8) & 0xFF, (lo >> 16) & 0xFF, (lo >> 24) & 0xFF,
                       hi & 0xFF, (hi >> 8) & 0xFF, (hi >> 16) & 0xFF, (hi >> 24) & 0xFF)
        self.pos = p + 8
    end

    function self:write_uint32(v) self:write_int32(v) end
    function self:write_uint64(v) self:write_int64(v) end

    function self:write_float32(v)
        self:ensure_capacity(4)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        local bits = string.unpack("I4", string.pack("f", v))
        self.chunks[#self.chunks] = current:sub(1, p) ..
            string.char(bits & 0xFF, (bits >> 8) & 0xFF, (bits >> 16) & 0xFF, (bits >> 24) & 0xFF)
        self.pos = p + 4
    end

    function self:write_float64(v)
        self:ensure_capacity(8)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        local bits_lo, bits_hi = string.unpack("I4I4", string.pack("d", v))
        self.chunks[#self.chunks] = current:sub(1, p) ..
            string.char(bits_lo & 0xFF, (bits_lo >> 8) & 0xFF, (bits_lo >> 16) & 0xFF, (bits_lo >> 24) & 0xFF,
                       bits_hi & 0xFF, (bits_hi >> 8) & 0xFF, (bits_hi >> 16) & 0xFF, (bits_hi >> 24) & 0xFF)
        self.pos = p + 8
    end

    function self:write_compact_length(len)
        if len <= xpb.COMPACT_LENGTH_THRESHOLD then
            self:ensure_capacity(1)
            local current = self.chunks[#self.chunks]
            self.pos = self.pos + 1
            self.chunks[#self.chunks] = current:sub(1, self.pos - 1) .. string.char(len)
        else
            self:ensure_capacity(5)
            local current = self.chunks[#self.chunks]
            local p = self.pos
            self.chunks[#self.chunks] = current:sub(1, p) ..
                string.char(xpb.COMPACT_LENGTH_MARKER, len & 0xFF, (len >> 8) & 0xFF, (len >> 16) & 0xFF, (len >> 24) & 0xFF)
            self.pos = p + 5
        end
    end

    function self:write_string(v)
        self:write_compact_length(#v)
        if #v == 0 then return end
        self:ensure_capacity(#v)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        self.chunks[#self.chunks] = current:sub(1, p) .. v
        self.pos = p + #v
    end

    function self:write_bytes(data)
        self:write_compact_length(#data)
        if #data == 0 then return end
        self:ensure_capacity(#data)
        local current = self.chunks[#self.chunks]
        local p = self.pos
        self.chunks[#self.chunks] = current:sub(1, p) .. data
        self.pos = p + #data
    end

    function self:write_message(data)
        self:write_bytes(data)
    end

    function self:finish()
        local used = self.chunks[#self.chunks]:sub(1, self.pos)
        self.chunks[#self.chunks] = used
        return table.concat(self.chunks)
    end

    function self:reset()
        self.pos = 0
        self.chunks[#self.chunks] = string.rep("\0", self.chunk_size)
    end

    return self
end

-- Decoder
function xpb.Decoder(data)
    local self = {
        data = data,
        len = #data,
        pos = 1
    }

    function self:eof()
        return self.pos > self.len
    end

    function self:remaining()
        return self.len - self.pos + 1
    end

    function self:read_bool()
        local v = self.data:byte(self.pos) ~= 0
        self.pos = self.pos + 1
        return v
    end

    function self:read_int32()
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        local n = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
        if n >= 2147483648 then n = n - 4294967296 end
        self.pos = self.pos + 4
        return n
    end

    function self:read_int64()
        local b1, b2, b3, b4, b5, b6, b7, b8 = self.data:byte(self.pos, self.pos + 7)
        local lo = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
        local hi = b5 + (b6 << 8) + (b7 << 16) + (b8 << 24)
        local n = lo + (hi << 32)
        if n >= 9223372036854775808 then n = n - 18446744073709551616 end
        self.pos = self.pos + 8
        return n
    end

    function self:read_uint32() return self:read_int32() end
    function self:read_uint64() return self:read_int64() end

    function self:read_float32()
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        return string.unpack("f", string.char(b1, b2, b3, b4))
    end

    function self:read_float64()
        local b1, b2, b3, b4, b5, b6, b7, b8 = self.data:byte(self.pos, self.pos + 7)
        self.pos = self.pos + 8
        return string.unpack("d", string.char(b1, b2, b3, b4, b5, b6, b7, b8))
    end

    function self:read_compact_length()
        local first = self.data:byte(self.pos)
        self.pos = self.pos + 1
        if first ~= xpb.COMPACT_LENGTH_MARKER then
            return first
        end
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        return b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
    end

    function self:read_string()
        local len = self:read_compact_length()
        if len == 0 then return "" end
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_bytes()
        local len = self:read_compact_length()
        if len == 0 then return "" end
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_message_bytes()
        return self:read_bytes()
    end

    function self:skip(n)
        self.pos = self.pos + n
    end

    return self
end

return xpb
