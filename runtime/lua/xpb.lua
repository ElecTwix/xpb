-- XPB V2 Lua Runtime
-- Pure Lua implementation of XPB V2 binary serialization

local xpb = {}

-- Constants
xpb.COMPACT_LENGTH_THRESHOLD = 254
xpb.COMPACT_LENGTH_MARKER = 0xFF

-- Utility functions
local function le32_to_num(bytes)
    if #bytes < 4 then return 0 end
    return bytes:byte(1) +
           (bytes:byte(2) * 256) +
           (bytes:byte(3) * 65536) +
           (bytes:byte(4) * 16777216)
end

local function le64_to_num(bytes)
    if #bytes < 8 then return 0 end
    local lo = bytes:byte(1) +
               (bytes:byte(2) * 256) +
               (bytes:byte(3) * 65536) +
               (bytes:byte(4) * 16777216)
    local hi = bytes:byte(5) +
               (bytes:byte(6) * 256) +
               (bytes:byte(7) * 65536) +
               (bytes:byte(8) * 16777216)
    return lo + (hi * 4294967296)
end

local function num_to_le32(n)
    return string.char(
        n & 0xFF,
        (n >> 8) & 0xFF,
        (n >> 16) & 0xFF,
        (n >> 24) & 0xFF
    )
end

local function num_to_le64(n)
    local lo = n & 0xFFFFFFFF
    local hi = math.floor(n / 4294967296) & 0xFFFFFFFF
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

-- Encoder
function xpb.Encoder(initial_size)
    local self = {
        buf = {},
        pos = 1
    }
    if initial_size then
        for i = 1, initial_size do
            self.buf[i] = string.char(0)
        end
    end

    function self:ensure_capacity(needed)
        while #self.buf - self.pos + 1 < needed do
            self.buf[#self.buf + 1] = string.char(0)
            self.buf[#self.buf + 1] = string.char(0)
            self.buf[#self.buf + 1] = string.char(0)
            self.buf[#self.buf + 1] = string.char(0)
        end
    end

    function self:write_bool(v)
        self:ensure_capacity(1)
        self.buf[self.pos] = v and string.char(1) or string.char(0)
        self.pos = self.pos + 1
    end

    function self:write_int32(v)
        self:ensure_capacity(4)
        local bytes = num_to_le32(v % 4294967296)
        if v < 0 then bytes = num_to_le32(4294967296 + v) end
        for i = 1, 4 do
            self.buf[self.pos + i - 1] = bytes:sub(i, i)
        end
        self.pos = self.pos + 4
    end

    function self:write_int64(v)
        self:ensure_capacity(8)
        local bytes = num_to_le64(v)
        for i = 1, 8 do
            self.buf[self.pos + i - 1] = bytes:sub(i, i)
        end
        self.pos = self.pos + 8
    end

    function self:write_uint32(v)
        self:write_int32(v)
    end

    function self:write_uint64(v)
        self:write_int64(v)
    end

    function self:write_float32(v)
        -- Convert float to bits
        local bytes = string.pack("f", v)
        self:ensure_capacity(4)
        for i = 1, 4 do
            self.buf[self.pos + i - 1] = bytes:sub(i, i)
        end
        self.pos = self.pos + 4
    end

    function self:write_float64(v)
        local bytes = string.pack("d", v)
        self:ensure_capacity(8)
        for i = 1, 8 do
            self.buf[self.pos + i - 1] = bytes:sub(i, i)
        end
        self.pos = self.pos + 8
    end

    function self:write_compact_length(len)
        if len <= xpb.COMPACT_LENGTH_THRESHOLD then
            self:ensure_capacity(1)
            self.buf[self.pos] = string.char(len)
            self.pos = self.pos + 1
        else
            self:ensure_capacity(5)
            self.buf[self.pos] = string.char(xpb.COMPACT_LENGTH_MARKER)
            self.pos = self.pos + 1
            local len_bytes = num_to_le32(len)
            for i = 1, 4 do
                self.buf[self.pos + i - 1] = len_bytes:sub(i, i)
            end
            self.pos = self.pos + 4
        end
    end

    function self:write_string(v)
        local bytes = {string.char(#v)}
        for i = 1, #v do
            bytes[i + 1] = v:sub(i, i)
        end
        self:write_compact_length(#v)
        self:ensure_capacity(#v)
        for i = 1, #v do
            self.buf[self.pos + i - 1] = v:sub(i, i)
        end
        self.pos = self.pos + #v
    end

    function self:write_bytes(data)
        self:write_compact_length(#data)
        self:ensure_capacity(#data)
        for i = 1, #data do
            self.buf[self.pos + i - 1] = data:sub(i, i)
        end
        self.pos = self.pos + #data
    end

    function self:write_message(data)
        self:write_bytes(data)
    end

    function self:finish()
        local result = ""
        for i = 1, self.pos - 1 do
            result = result .. (self.buf[i] or string.char(0))
        end
        return result
    end

    function self:reset()
        self.pos = 1
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
        if self.pos > self.len then
            error("xpb: unexpected EOF reading bool")
        end
        local v = self.data:byte(self.pos) ~= 0
        self.pos = self.pos + 1
        return v
    end

    function self:read_int32()
        if self.pos + 3 > self.len then
            error("xpb: unexpected EOF reading int32")
        end
        local bytes = self.data:sub(self.pos, self.pos + 3)
        local v = le32_to_num(bytes)
        self.pos = self.pos + 4
        return v
    end

    function self:read_int64()
        if self.pos + 7 > self.len then
            error("xpb: unexpected EOF reading int64")
        end
        local bytes = self.data:sub(self.pos, self.pos + 7)
        local v = le64_to_num(bytes)
        self.pos = self.pos + 8
        return v
    end

    function self:read_uint32()
        return self:read_int32()
    end

    function self:read_uint64()
        return self:read_int64()
    end

    function self:read_float32()
        if self.pos + 3 > self.len then
            error("xpb: unexpected EOF reading float32")
        end
        local bytes = self.data:sub(self.pos, self.pos + 3)
        local v = string.unpack("f", bytes)
        self.pos = self.pos + 4
        return v
    end

    function self:read_float64()
        if self.pos + 7 > self.len then
            error("xpb: unexpected EOF reading float64")
        end
        local bytes = self.data:sub(self.pos, self.pos + 7)
        local v = string.unpack("d", bytes)
        self.pos = self.pos + 8
        return v
    end

    function self:read_compact_length()
        if self.pos > self.len then
            error("xpb: unexpected EOF reading length")
        end
        local first = self.data:byte(self.pos)
        self.pos = self.pos + 1
        if first ~= xpb.COMPACT_LENGTH_MARKER then
            return first
        end
        if self.pos + 3 > self.len then
            error("xpb: unexpected EOF reading extended length")
        end
        local len_bytes = self.data:sub(self.pos, self.pos + 3)
        local len = le32_to_num(len_bytes)
        self.pos = self.pos + 4
        return len
    end

    function self:read_string()
        local len = self:read_compact_length()
        if self.pos + len - 1 > self.len then
            error("xpb: unexpected EOF reading string")
        end
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_bytes()
        local len = self:read_compact_length()
        if self.pos + len - 1 > self.len then
            error("xpb: unexpected EOF reading bytes")
        end
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_message_bytes()
        return self:read_bytes()
    end

    function self:skip(n)
        if self.pos + n - 1 > self.len then
            error("xpb: unexpected EOF during skip")
        end
        self.pos = self.pos + n
    end

    return self
end

return xpb
