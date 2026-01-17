-- XPB V2 Lua Runtime (Maximum Performance)
-- Pure Lua implementation of XPB V2 binary serialization

local xpb = {}

-- Constants
xpb.COMPACT_LENGTH_THRESHOLD = 254
xpb.COMPACT_LENGTH_MARKER = 0xFF

-- Fast bit operations
local function le32_to_num(b1, b2, b3, b4)
    local n = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
    if n >= 2147483648 then n = n - 4294967296 end
    return n
end

local function le64_to_num(b1, b2, b3, b4, b5, b6, b7, b8)
    local lo = b1 + (b2 << 8) + (b3 << 16) + (b4 << 24)
    local hi = b5 + (b6 << 8) + (b7 << 16) + (b8 << 24)
    local n = lo + (hi << 32)
    if n >= 9223372036854775808 then n = n - 18446744073709551616 end
    return n
end

-- Encoder (table-based byte buffer)
function xpb.Encoder(initial_size)
    local self = {
        buf = {},
        pos = 0
    }
    local buf = self.buf

    function self:ensure_capacity(needed)
        while #buf < self.pos + needed do
            buf[#buf + 1] = 0
        end
    end

    function self:write_byte(b)
        self.pos = self.pos + 1
        buf[self.pos] = b
    end

    function self:write_bool(v)
        self.pos = self.pos + 1
        buf[self.pos] = v and 1 or 0
    end

    function self:write_int32(v)
        self:ensure_capacity(4)
        local p = self.pos + 1
        buf[p] = v & 0xFF
        buf[p + 1] = (v >> 8) & 0xFF
        buf[p + 2] = (v >> 16) & 0xFF
        buf[p + 3] = (v >> 24) & 0xFF
        self.pos = self.pos + 4
    end

    function self:write_int64(v)
        self:ensure_capacity(8)
        local p = self.pos + 1
        local lo = v & 0xFFFFFFFF
        local hi = (v >> 32) & 0xFFFFFFFF
        buf[p] = lo & 0xFF
        buf[p + 1] = (lo >> 8) & 0xFF
        buf[p + 2] = (lo >> 16) & 0xFF
        buf[p + 3] = (lo >> 24) & 0xFF
        buf[p + 4] = hi & 0xFF
        buf[p + 5] = (hi >> 8) & 0xFF
        buf[p + 6] = (hi >> 16) & 0xFF
        buf[p + 7] = (hi >> 24) & 0xFF
        self.pos = self.pos + 8
    end

    function self:write_uint32(v) self:write_int32(v) end
    function self:write_uint64(v) self:write_int64(v) end

    function self:write_float32(v)
        self:ensure_capacity(4)
        local bits = string.unpack("I4", string.pack("f", v))
        local p = self.pos + 1
        buf[p] = bits & 0xFF
        buf[p + 1] = (bits >> 8) & 0xFF
        buf[p + 2] = (bits >> 16) & 0xFF
        buf[p + 3] = (bits >> 24) & 0xFF
        self.pos = self.pos + 4
    end

    function self:write_float64(v)
        self:ensure_capacity(8)
        local bits_lo, bits_hi = string.unpack("I4I4", string.pack("d", v))
        local p = self.pos + 1
        buf[p] = bits_lo & 0xFF
        buf[p + 1] = (bits_lo >> 8) & 0xFF
        buf[p + 2] = (bits_lo >> 16) & 0xFF
        buf[p + 3] = (bits_lo >> 24) & 0xFF
        buf[p + 4] = bits_hi & 0xFF
        buf[p + 5] = (bits_hi >> 8) & 0xFF
        buf[p + 6] = (bits_hi >> 16) & 0xFF
        buf[p + 7] = (bits_hi >> 24) & 0xFF
        self.pos = self.pos + 8
    end

    function self:write_compact_length(len)
        if len <= xpb.COMPACT_LENGTH_THRESHOLD then
            self:ensure_capacity(1)
            self.pos = self.pos + 1
            buf[self.pos] = len
        else
            self:ensure_capacity(5)
            local p = self.pos + 1
            buf[p] = xpb.COMPACT_LENGTH_MARKER
            buf[p + 1] = len & 0xFF
            buf[p + 2] = (len >> 8) & 0xFF
            buf[p + 3] = (len >> 16) & 0xFF
            buf[p + 4] = (len >> 24) & 0xFF
            self.pos = self.pos + 5
        end
    end

    function self:write_string(v)
        self:write_compact_length(#v)
        if #v == 0 then return end
        self:ensure_capacity(#v)
        local p = self.pos + 1
        for i = 1, #v do
            buf[p + i - 1] = v:byte(i)
        end
        self.pos = self.pos + #v
    end

    function self:write_bytes(data)
        self:write_compact_length(#data)
        if #data == 0 then return end
        self:ensure_capacity(#data)
        local p = self.pos + 1
        for i = 1, #data do
            buf[p + i - 1] = data:byte(i)
        end
        self.pos = self.pos + #data
    end

    function self:write_message(data)
        self:write_bytes(data)
    end

    function self:finish()
        if self.pos == 0 then return "" end
        local result = {}
        for i = 1, self.pos do
            result[i] = buf[i]
        end
        return string.char(table.unpack(result))
    end

    function self:reset()
        self.pos = 0
    end

    return self
end

-- Decoder (optimized)
function xpb.Decoder(data)
    local self = {
        data = data,
        len = #data,
        pos = 1
    }

    function self:eof() return self.pos > self.len end
    function self:remaining() return self.len - self.pos + 1 end

    function self:read_bool()
        local v = self.data:byte(self.pos) ~= 0
        self.pos = self.pos + 1
        return v
    end

    function self:read_int32()
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        return le32_to_num(b1, b2, b3, b4)
    end

    function self:read_int64()
        local b1, b2, b3, b4, b5, b6, b7, b8 = self.data:byte(self.pos, self.pos + 7)
        self.pos = self.pos + 8
        return le64_to_num(b1, b2, b3, b4, b5, b6, b7, b8)
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
        return le32_to_num(b1, b2, b3, b4)
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

    function self:read_message_bytes() return self:read_bytes() end
    function self:skip(n) self.pos = self.pos + n end

    return self
end

return xpb
