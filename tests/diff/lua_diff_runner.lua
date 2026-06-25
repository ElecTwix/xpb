-- Cross-language DIFFERENTIAL runner for the Lua runtime (T-9).
--
-- This is the Lua arm of the random cross-language differential fuzzer in
-- tests/differential. It is a standalone script (NOT the committed
-- tests/lua/conformance.lua, which is left untouched) so the Go driver can point
-- it at an arbitrary corpus directory passed as arg[1].
--
-- Usage:  lua lua_diff_runner.lua <corpus-dir> [bytes|values]
--
-- It reuses the runtime library (runtime/lua/xpb.lua) and mirrors the
-- conformance harness's JSON parsing + op encode/verify: decode each .bin with
-- the Lua runtime and verify decoded values bit-exactly. In `bytes` mode
-- (default; the map-FREE corpus) it then re-encodes and asserts byte-identity
-- with the Go reference .bin. In `values` mode (the map-CONTAINING corpus) the
-- byte-identity check is skipped, because map entry order is non-canonical
-- across runtimes (T-7). Exits non-zero on any mismatch.
--
-- Requires Lua 5.3+ (string.pack/unpack, integer division, bitwise ops).

local corpus_dir = arg and arg[1]
if not corpus_dir then
    io.stderr:write("usage: lua lua_diff_runner.lua <corpus-dir> [bytes|values]\n")
    os.exit(2)
end
local values_only = (arg and arg[2] == "values")

-- Resolve the runtime module relative to this script (tests/diff/), repo root is
-- two directories up.
local script_path = arg and arg[0] or "tests/diff/lua_diff_runner.lua"
local script_dir = script_path:match("^(.*)[/\\][^/\\]*$") or "."
local repo_root = script_dir .. "/../.."
package.path = repo_root .. "/runtime/lua/?.lua;" .. package.path
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
                            -- Reject a low surrogate outside [0xDC00,0xDFFF].
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

local function bytes_to_hex(b)
    local parts = {}
    for i = 1, #b do
        parts[i] = string.format("%02x", b:byte(i))
    end
    return table.concat(parts)
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
-- Encode / verify ops
-- ---------------------------------------------------------------------------

local encode_op, encode_ops, verify_op, verify_ops

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

local function new_ctx(name) return { name = name, ok = true, msgs = {} } end
local function fail(ctx, path, msg)
    ctx.ok = false
    ctx.msgs[#ctx.msgs + 1] = string.format("    %s: %s", path, msg)
end
local function check(ctx, path, cond, msg)
    if not cond then fail(ctx, path, msg) end
end

function verify_ops(dec, ops, path, ctx)
    for i, op in ipairs(ops or {}) do
        verify_op(dec, op, string.format("%s[%d]", path, i), ctx)
    end
end

function verify_op(dec, op, path, ctx)
    local ty = op.type
    if ty == "bool" then
        check(ctx, path, dec:read_bool() == op.bool, "bool mismatch")
    elseif ty == "int32" then
        local got = dec:read_int32()
        local want = math.tointeger(op.int32) or math.floor(op.int32)
        check(ctx, path, got == want, string.format("int32 got %d want %d", got, want))
    elseif ty == "uint32" then
        local got = dec:read_uint32()
        local want = math.tointeger(op.uint32) or math.floor(op.uint32)
        check(ctx, path, (got & 0xFFFFFFFF) == (want & 0xFFFFFFFF),
            string.format("uint32 bits got %08x want %08x", got & 0xFFFFFFFF, want & 0xFFFFFFFF))
    elseif ty == "int64" then
        local got = dec:read_int64()
        local want = parse_int64(op.int64)
        check(ctx, path, got == want, string.format("int64 got %d want %d", got, want))
    elseif ty == "uint64" then
        local got = dec:read_uint64()
        local want = parse_int64(op.uint64)
        check(ctx, path, got == want, "uint64 bit mismatch")
    elseif ty == "float32" then
        local got = dec:read_float32()
        local got_bytes = string.pack("f", got)
        local want_bytes = string.pack("I4", tonumber(strip_hex_prefix(op.floatBits), 16) & 0xFFFFFFFF)
        check(ctx, path, got_bytes == want_bytes, "float32 bit mismatch")
    elseif ty == "float64" then
        local got = dec:read_float64()
        local got_bytes = string.pack("d", got)
        local want_bytes = string.pack("I8", tonumber(strip_hex_prefix(op.floatBits), 16))
        check(ctx, path, got_bytes == want_bytes, "float64 bit mismatch")
    elseif ty == "string" then
        check(ctx, path, dec:read_string() == (op.string or ""), "string mismatch")
    elseif ty == "bytes" then
        check(ctx, path, dec:read_bytes() == hex_to_bytes(op.bytes), "bytes mismatch")
    elseif ty == "array" then
        local count = dec:read_int32()
        local elems = op.elems or {}
        check(ctx, path, count == #elems, string.format("array count got %d want %d", count, #elems))
        for i, el in ipairs(elems) do
            verify_op(dec, el, string.format("%s.elem[%d]", path, i), ctx)
        end
    elseif ty == "map" then
        local count = dec:read_int32()
        local entries = op.entries or {}
        check(ctx, path, count == #entries, string.format("map count got %d want %d", count, #entries))
        for i, ent in ipairs(entries) do
            verify_op(dec, ent.k, string.format("%s.key[%d]", path, i), ctx)
            verify_op(dec, ent.v, string.format("%s.val[%d]", path, i), ctx)
        end
    elseif ty == "message" then
        local msg = dec:read_message_bytes()
        local inner = xpb.Decoder(msg)
        verify_ops(inner, op.ops, path .. ".msg", ctx)
        check(ctx, path, inner:eof(), "nested message trailing bytes")
    else
        fail(ctx, path, "unknown op type: " .. tostring(ty))
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

local passed, failed = 0, 0
for _, v in ipairs(vectors) do
    local ctx = new_ctx(v.name)
    local file_bytes = read_file(corpus_dir .. "/" .. v.file)
    if not file_bytes then
        fail(ctx, v.name, "missing .bin file: " .. v.file)
    else
        local ok, derr = pcall(function()
            local dec = xpb.Decoder(file_bytes)
            verify_ops(dec, v.ops, v.name, ctx)
            check(ctx, v.name, dec:eof(), "trailing bytes after decode")
        end)
        if not ok then fail(ctx, v.name, "decode error: " .. tostring(derr)) end

        if not values_only then
            -- Byte mode (map-free corpus): re-encode and assert byte-identity.
            local eok, reencoded = pcall(function()
                local enc = xpb.Encoder(256)
                encode_ops(enc, v.ops)
                return enc:finish()
            end)
            if not eok then
                fail(ctx, v.name, "encode error: " .. tostring(reencoded))
            else
                check(ctx, v.name, reencoded == file_bytes, string.format(
                    "re-encode mismatch\n      got:  %s\n      want: %s",
                    bytes_to_hex(reencoded), bytes_to_hex(file_bytes)))
            end
        else
            -- Values mode (map-containing corpus): byte order is non-canonical,
            -- so exercise this runtime's ENCODER via a self round-trip: re-encode
            -- the values, decode that back, and re-verify the values.
            local rok, rerr = pcall(function()
                local enc = xpb.Encoder(256)
                encode_ops(enc, v.ops)
                local rdec = xpb.Decoder(enc:finish())
                verify_ops(rdec, v.ops, v.name, ctx)
                check(ctx, v.name, rdec:eof(), "self round-trip trailing bytes")
            end)
            if not rok then fail(ctx, v.name, "self round-trip error: " .. tostring(rerr)) end
        end
    end

    if ctx.ok then
        passed = passed + 1
    else
        print(string.format("  [FAIL] %s", v.name))
        for _, m in ipairs(ctx.msgs) do print(m) end
        failed = failed + 1
    end
end

print(string.format("Lua differential (%s): %d vectors verified, %d failed",
    values_only and "values" or "bytes", passed, failed))
os.exit(failed > 0 and 1 or 0)
