-- Timed cross-runtime benchmark harness for the Lua runtime (xpbench / T-17).
--
-- This is the Lua arm of the cross-runtime benchmark TABLE driven by
-- cmd/xpbench. It is the timed analogue of the proven differential runner
-- (tests/diff/lua_diff_runner.lua): it reads the shared vectors.json manifest +
-- .bin corpus the Go reference encoder wrote, then for every vector times an
-- encode loop (re-encode the ops with the Lua Encoder) and a decode loop (decode
-- the .bin bytes with the Lua Decoder) over a per-vector iteration count, and
-- prints a JSON array of {name, encodeNs, decodeNs, wireSize} to stdout for the
-- Go driver to normalize into table rows.
--
-- Usage:  lua lua_bench.lua <corpus-dir> <runtime-lua-dir>
--
-- Requires Lua 5.3+ (string.pack/unpack, integer division, bitwise ops).
--
-- Timing note: Lua's stdlib has no high-resolution wall clock, so this harness
-- times with os.clock() (process CPU time). The benchmark loop is single-
-- threaded and CPU-bound, so CPU time tracks wall time closely; still, the Lua
-- row's ns/op is CPU-time-derived, whereas the other runtimes use a wall-clock
-- monotonic source -- read the Lua numbers as same-order-of-magnitude, not as a
-- nanosecond-exact peer of the compiled rows.

local corpus_dir = arg and arg[1]
local lua_dir = arg and arg[2]
if not corpus_dir or not lua_dir then
    io.stderr:write("usage: lua lua_bench.lua <corpus-dir> <runtime-lua-dir>\n")
    os.exit(2)
end

package.path = lua_dir .. "/?.lua;" .. package.path
local xpb = require "xpb"

-- ---------------------------------------------------------------------------
-- Minimal JSON parser (objects, arrays, strings, numbers, bool, null).
-- ---------------------------------------------------------------------------
local function json_decode(s)
    local pos = 1
    local len = #s
    local value

    local function err(msg)
        error(string.format("json: %s at byte %d", msg, pos), 2)
    end

    local function skip_ws()
        while pos <= len do
            local c = s:byte(pos)
            if c == 32 or c == 9 or c == 10 or c == 13 then
                pos = pos + 1
            else
                break
            end
        end
    end

    local function parse_string()
        pos = pos + 1
        local parts = {}
        while pos <= len do
            local c = s:byte(pos)
            if c == 34 then
                pos = pos + 1
                return table.concat(parts)
            elseif c == 92 then
                local e = s:byte(pos + 1)
                if e == 110 then parts[#parts + 1] = "\n"
                elseif e == 116 then parts[#parts + 1] = "\t"
                elseif e == 114 then parts[#parts + 1] = "\r"
                elseif e == 98 then parts[#parts + 1] = "\b"
                elseif e == 102 then parts[#parts + 1] = "\f"
                elseif e == 47 then parts[#parts + 1] = "/"
                elseif e == 92 then parts[#parts + 1] = "\\"
                elseif e == 34 then parts[#parts + 1] = "\""
                elseif e == 117 then
                    local hex = s:sub(pos + 2, pos + 5)
                    local cp = tonumber(hex, 16)
                    if not cp then err("bad \\u escape") end
                    pos = pos + 4
                    if cp >= 0xD800 and cp <= 0xDBFF then
                        if s:byte(pos + 2) == 92 and s:byte(pos + 3) == 117 then
                            local lo = tonumber(s:sub(pos + 4, pos + 7), 16)
                            if not lo or lo < 0xDC00 or lo > 0xDFFF then
                                err("invalid low surrogate")
                            end
                            cp = 0x10000 + ((cp - 0xD800) * 0x400) + (lo - 0xDC00)
                            pos = pos + 6
                        end
                    end
                    parts[#parts + 1] = utf8.char(cp)
                else
                    err("bad escape")
                end
                pos = pos + 2
            else
                local start = pos
                while pos <= len do
                    local cc = s:byte(pos)
                    if cc == 34 or cc == 92 then break end
                    pos = pos + 1
                end
                parts[#parts + 1] = s:sub(start, pos - 1)
            end
        end
        err("unterminated string")
    end

    local function parse_number()
        local start = pos
        while pos <= len do
            local c = s:byte(pos)
            if (c >= 48 and c <= 57) or c == 45 or c == 43
                or c == 46 or c == 101 or c == 69 then
                pos = pos + 1
            else
                break
            end
        end
        return tonumber(s:sub(start, pos - 1))
    end

    local function parse_array()
        pos = pos + 1
        local arr = {}
        skip_ws()
        if s:byte(pos) == 93 then pos = pos + 1; return arr end
        while true do
            skip_ws()
            arr[#arr + 1] = value()
            skip_ws()
            local c = s:byte(pos)
            if c == 44 then pos = pos + 1
            elseif c == 93 then pos = pos + 1; return arr
            else err("expected , or ] in array") end
        end
    end

    local function parse_object()
        pos = pos + 1
        local obj = {}
        skip_ws()
        if s:byte(pos) == 125 then pos = pos + 1; return obj end
        while true do
            skip_ws()
            if s:byte(pos) ~= 34 then err("expected string key") end
            local k = parse_string()
            skip_ws()
            if s:byte(pos) ~= 58 then err("expected : after key") end
            pos = pos + 1
            skip_ws()
            obj[k] = value()
            skip_ws()
            local c = s:byte(pos)
            if c == 44 then pos = pos + 1
            elseif c == 125 then pos = pos + 1; return obj
            else err("expected , or } in object") end
        end
    end

    value = function()
        skip_ws()
        local c = s:byte(pos)
        if c == 123 then return parse_object()
        elseif c == 91 then return parse_array()
        elseif c == 34 then return parse_string()
        elseif c == 116 then
            if s:sub(pos, pos + 3) == "true" then pos = pos + 4; return true end
            err("bad literal")
        elseif c == 102 then
            if s:sub(pos, pos + 4) == "false" then pos = pos + 5; return false end
            err("bad literal")
        elseif c == 110 then
            if s:sub(pos, pos + 3) == "null" then pos = pos + 4; return nil end
            err("bad literal")
        else
            return parse_number()
        end
    end

    skip_ws()
    return value()
end

-- ---------------------------------------------------------------------------
-- Helpers
-- ---------------------------------------------------------------------------

local function read_file(path)
    local f = io.open(path, "rb")
    if not f then return nil end
    local data = f:read("*a")
    f:close()
    return data
end

local function hex_to_bytes(hex)
    if hex == nil or hex == "" then return "" end
    local out = {}
    for i = 1, #hex, 2 do
        out[#out + 1] = tonumber(hex:sub(i, i + 1), 16)
    end
    return string.char(table.unpack(out))
end

local function strip_hex_prefix(s)
    if s:sub(1, 2) == "0x" or s:sub(1, 2) == "0X" then return s:sub(3) end
    return s
end

local function parse_int64(str)
    local neg = false
    local i = 1
    if str:sub(1, 1) == "-" then neg = true; i = 2
    elseif str:sub(1, 1) == "+" then i = 2 end
    local acc = 0
    while i <= #str do
        local d = str:byte(i) - 48
        if d < 0 or d > 9 then error("bad int64 digit in " .. str) end
        acc = acc * 10 + d
        i = i + 1
    end
    if neg then acc = -acc end
    return acc
end

local function float32_from_bits(hex)
    local bits = tonumber(strip_hex_prefix(hex), 16)
    return string.unpack("f", string.pack("I4", bits & 0xFFFFFFFF))
end

local function float64_from_bits(hex)
    local bits = tonumber(strip_hex_prefix(hex), 16)
    return string.unpack("d", string.pack("I8", bits))
end

-- ---------------------------------------------------------------------------
-- Encode / decode ops (the timed work)
-- ---------------------------------------------------------------------------

local encode_op, encode_ops, decode_op, decode_ops

function encode_ops(enc, ops)
    for _, op in ipairs(ops or {}) do encode_op(enc, op) end
end

function encode_op(enc, op)
    local ty = op.type
    if ty == "bool" then
        enc:write_bool(op.bool)
    elseif ty == "int32" then
        enc:write_int32(math.tointeger(op.int32) or math.floor(op.int32))
    elseif ty == "uint32" then
        enc:write_uint32(math.tointeger(op.uint32) or math.floor(op.uint32))
    elseif ty == "int64" then
        enc:write_int64(parse_int64(op.int64))
    elseif ty == "uint64" then
        enc:write_uint64(parse_int64(op.uint64))
    elseif ty == "float32" then
        enc:write_float32(float32_from_bits(op.floatBits))
    elseif ty == "float64" then
        enc:write_float64(float64_from_bits(op.floatBits))
    elseif ty == "string" then
        enc:write_string(op.string or "")
    elseif ty == "bytes" then
        enc:write_bytes(hex_to_bytes(op.bytes))
    elseif ty == "array" then
        local elems = op.elems or {}
        enc:write_int32(#elems)
        for _, el in ipairs(elems) do encode_op(enc, el) end
    elseif ty == "map" then
        local entries = op.entries or {}
        enc:write_int32(#entries)
        for _, ent in ipairs(entries) do
            encode_op(enc, ent.k)
            encode_op(enc, ent.v)
        end
    elseif ty == "message" then
        local inner = xpb.Encoder(64)
        encode_ops(inner, op.ops)
        enc:write_message(inner:finish())
    else
        error("unknown op type: " .. tostring(ty))
    end
end

-- decode_op reads every value (decode-only, matching the Go driver's
-- decode-only path and the other harnesses) and accumulates a small number so
-- the read cannot be elided.
function decode_ops(dec, ops)
    local acc = 0
    for _, op in ipairs(ops or {}) do acc = acc + decode_op(dec, op) end
    return acc
end

function decode_op(dec, op)
    local ty = op.type
    if ty == "bool" then
        return dec:read_bool() and 1 or 0
    elseif ty == "int32" then
        return dec:read_int32() & 0xFFFFFFFF
    elseif ty == "uint32" then
        return dec:read_uint32() & 0xFFFFFFFF
    elseif ty == "int64" then
        return dec:read_int64() & 0xFF
    elseif ty == "uint64" then
        return dec:read_uint64() & 0xFF
    elseif ty == "float32" then
        return math.floor(dec:read_float32()) & 0xFF
    elseif ty == "float64" then
        return math.floor(dec:read_float64()) & 0xFF
    elseif ty == "string" then
        return #dec:read_string()
    elseif ty == "bytes" then
        return #dec:read_bytes()
    elseif ty == "array" then
        local elems = op.elems or {}
        local count = dec:read_int32()
        local acc = count
        for _, el in ipairs(elems) do acc = acc + decode_op(dec, el) end
        return acc
    elseif ty == "map" then
        local entries = op.entries or {}
        local count = dec:read_int32()
        local acc = count
        for _, ent in ipairs(entries) do
            acc = acc + decode_op(dec, ent.k)
            acc = acc + decode_op(dec, ent.v)
        end
        return acc
    elseif ty == "message" then
        local msg = dec:read_message_bytes()
        local inner = xpb.Decoder(msg)
        return decode_ops(inner, op.ops) + #msg
    else
        error("unknown op type: " .. tostring(ty))
    end
end

-- ---------------------------------------------------------------------------
-- Main
-- ---------------------------------------------------------------------------

local raw = read_file(corpus_dir .. "/vectors.json")
if not raw then
    io.stderr:write("ERROR: cannot read " .. corpus_dir .. "/vectors.json\n")
    os.exit(2)
end

local manifest = json_decode(raw)
local vectors = manifest.vectors
if not vectors or #vectors == 0 then
    io.stderr:write("ERROR: manifest has no vectors\n")
    os.exit(1)
end

local sink = 0
local parts = {}
for vi, v in ipairs(vectors) do
    local file_bytes = read_file(corpus_dir .. "/" .. v.file)
    if not file_bytes then
        io.stderr:write("ERROR: missing .bin file: " .. tostring(v.file) .. "\n")
    else
        local wire = #file_bytes
        local iters = math.tointeger(v.iters) or math.floor(v.iters or 1)
        if iters < 1 then iters = 1 end
        local warm = iters // 10
        if warm < 1 then warm = 1 end

        -- Encode timing.
        for _ = 1, warm do
            local enc = xpb.Encoder(256)
            encode_ops(enc, v.ops)
            sink = sink + #enc:finish()
        end
        local t0 = os.clock()
        for _ = 1, iters do
            local enc = xpb.Encoder(256)
            encode_ops(enc, v.ops)
            sink = sink + #enc:finish()
        end
        local enc_ns = (os.clock() - t0) * 1e9 / iters

        -- Decode timing.
        for _ = 1, warm do
            local dec = xpb.Decoder(file_bytes)
            sink = sink + decode_ops(dec, v.ops)
        end
        local t1 = os.clock()
        for _ = 1, iters do
            local dec = xpb.Decoder(file_bytes)
            sink = sink + decode_ops(dec, v.ops)
        end
        local dec_ns = (os.clock() - t1) * 1e9 / iters

        parts[#parts + 1] = string.format(
            '{"name":"%s","encodeNs":%.3f,"decodeNs":%.3f,"wireSize":%d}',
            v.name, enc_ns, dec_ns, wire)
    end
end

io.write("[" .. table.concat(parts, ",") .. "]\n")
-- Reference the sink so the timed loops are not optimized away.
if sink < 0 then io.stderr:write("sink=" .. tostring(sink) .. "\n") end
