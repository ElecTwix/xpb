-- XPB V2 Lua Runtime (Maximum Performance)
-- Pure Lua implementation of XPB V2 binary serialization

local xpb = {}

-- Constants
xpb.COMPACT_LENGTH_THRESHOLD = 254
xpb.COMPACT_LENGTH_MARKER = 0xFF

-- Cap on nested-message decode recursion. Mirrors xpb.MaxDecodeDepth in
-- the Go runtime and MaxDecodeDepth in the TS runtime; the generated
-- UnmarshalAt(data, depth) shim compares against it before doing any
-- work. Without this cap, an adversarial deeply-nested payload drives
-- the Lua C stack into a fatal overflow.
xpb.MAX_DECODE_DEPTH = 64

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

    function self:write_array_int32(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_int32(arr[i])
        end
    end

    function self:write_array_int64(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_int64(arr[i])
        end
    end

    function self:write_array_uint32(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_uint32(arr[i])
        end
    end

    function self:write_array_uint64(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_uint64(arr[i])
        end
    end

    function self:write_array_float32(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_float32(arr[i])
        end
    end

    function self:write_array_float64(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_float64(arr[i])
        end
    end

    function self:write_array_bool(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_bool(arr[i])
        end
    end

    function self:write_array_string(arr)
        self:write_int32(#arr)
        for i = 1, #arr do
            self:write_string(arr[i])
        end
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
--
-- Every read funnels through ensure_bytes() so a single malformed length
-- can't run off the end of the buffer. Array reads require the caller to
-- pass an explicit max_elements budget: the runtime does NOT pick a default
-- because policy is a per-call-site decision.
function xpb.Decoder(data)
    local self = {
        data = data,
        len = #data,
        pos = 1
    }

    function self:eof() return self.pos > self.len end
    function self:remaining() return self.len - self.pos + 1 end

    -- Ensure n more bytes are readable; error otherwise.
    function self:ensure_bytes(n, what)
        if self.pos + n - 1 > self.len then
            error(string.format("xpb: unexpected EOF reading %s (need %d bytes, have %d)",
                what, n, self.len - self.pos + 1), 2)
        end
    end

    function self:read_bool()
        self:ensure_bytes(1, "bool")
        local v = self.data:byte(self.pos) ~= 0
        self.pos = self.pos + 1
        return v
    end

    function self:read_int32()
        self:ensure_bytes(4, "int32")
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        return le32_to_num(b1, b2, b3, b4)
    end

    function self:read_int64()
        self:ensure_bytes(8, "int64")
        local b1, b2, b3, b4, b5, b6, b7, b8 = self.data:byte(self.pos, self.pos + 7)
        self.pos = self.pos + 8
        return le64_to_num(b1, b2, b3, b4, b5, b6, b7, b8)
    end

    function self:read_uint32() return self:read_int32() end
    function self:read_uint64() return self:read_int64() end

    function self:read_float32()
        self:ensure_bytes(4, "float32")
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        return string.unpack("f", string.char(b1, b2, b3, b4))
    end

    function self:read_float64()
        self:ensure_bytes(8, "float64")
        local b1, b2, b3, b4, b5, b6, b7, b8 = self.data:byte(self.pos, self.pos + 7)
        self.pos = self.pos + 8
        return string.unpack("d", string.char(b1, b2, b3, b4, b5, b6, b7, b8))
    end

    function self:read_compact_length()
        self:ensure_bytes(1, "compact length")
        local first = self.data:byte(self.pos)
        self.pos = self.pos + 1
        if first ~= xpb.COMPACT_LENGTH_MARKER then
            return first
        end
        self:ensure_bytes(4, "extended length")
        local b1, b2, b3, b4 = self.data:byte(self.pos, self.pos + 3)
        self.pos = self.pos + 4
        local v = le32_to_num(b1, b2, b3, b4)
        if v < 0 then
            error("xpb: negative or oversized compact length", 2)
        end
        return v
    end

    function self:read_string()
        local len = self:read_compact_length()
        if len == 0 then return "" end
        self:ensure_bytes(len, "string")
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_bytes()
        local len = self:read_compact_length()
        if len == 0 then return "" end
        self:ensure_bytes(len, "bytes")
        local v = self.data:sub(self.pos, self.pos + len - 1)
        self.pos = self.pos + len
        return v
    end

    function self:read_message_bytes() return self:read_bytes() end

    function self:skip(n)
        self:ensure_bytes(n, "skip")
        self.pos = self.pos + n
    end

    -- Validate and return an array length read from the wire.
    -- The caller MUST pass max_elements — the runtime does not pick a
    -- default budget. element_min_bytes is the smallest possible on-wire
    -- size of one element (4 for int32, 1 for bool / variable-length).
    -- Pass 0 for element_min_bytes to skip the buffer-bound check.
    function self:read_array_count(element_min_bytes, max_elements)
        if max_elements == nil or max_elements < 0 then
            error("xpb: read_array_count requires non-negative max_elements", 2)
        end
        local n = self:read_int32()
        if n < 0 then
            error("xpb: negative array count: " .. n, 2)
        end
        if n > max_elements then
            error(string.format("xpb: array count %d exceeds caller-supplied max %d",
                n, max_elements), 2)
        end
        if element_min_bytes > 0 then
            local remaining = self.len - self.pos + 1
            local maxBuf = remaining // element_min_bytes
            if n > maxBuf then
                error(string.format("xpb: array count %d exceeds buffer-bounded max %d",
                    n, maxBuf), 2)
            end
        end
        return n
    end

    -- Array helpers. Every call REQUIRES the caller to pass max_elements;
    -- it caps the wire-supplied count before any per-element work runs.

    function self:read_array_int32(max_elements)
        local count = self:read_array_count(4, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_int32()
        end
        return arr
    end

    function self:read_array_int64(max_elements)
        local count = self:read_array_count(8, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_int64()
        end
        return arr
    end

    function self:read_array_uint32(max_elements)
        local count = self:read_array_count(4, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_uint32()
        end
        return arr
    end

    function self:read_array_uint64(max_elements)
        local count = self:read_array_count(8, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_uint64()
        end
        return arr
    end

    function self:read_array_float32(max_elements)
        local count = self:read_array_count(4, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_float32()
        end
        return arr
    end

    function self:read_array_float64(max_elements)
        local count = self:read_array_count(8, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_float64()
        end
        return arr
    end

    function self:read_array_bool(max_elements)
        local count = self:read_array_count(1, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_bool()
        end
        return arr
    end

    function self:read_array_string(max_elements)
        local count = self:read_array_count(1, max_elements)
        local arr = {}
        for i = 1, count do
            arr[i] = self:read_string()
        end
        return arr
    end

    return self
end

return xpb
