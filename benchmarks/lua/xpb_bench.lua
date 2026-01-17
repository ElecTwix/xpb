-- XPB V2 Lua Benchmark - Compare to JSON (cjson)
--
-- Run:
--   lua5.4 xpb_bench.lua

local xpb = require "xpb"

local ITERATIONS = 50000
local WARMUP = 100

local function time_ms(fn)
    for i = 1, WARMUP do fn() end
    local start = os.clock() * 1000
    for i = 1, WARMUP do fn() end
    local end_time = os.clock() * 1000
    return (end_time - start) / WARMUP
end

local function benchmark(fn)
    local start = os.clock() * 1000
    for i = 1, ITERATIONS do fn() end
    local end_time = os.clock() * 1000
    return (end_time - start) * 1000000 / ITERATIONS
end

local current_user = {}

local function xpb_encode()
    local enc = xpb.Encoder(64)
    enc:write_string("Alice Johnson")
    enc:write_int32(30)
    enc:write_bool(true)
    local data = enc:finish()
    return data
end

local function xpb_decode()
    local enc = xpb.Encoder(64)
    enc:write_string("Alice Johnson")
    enc:write_int32(30)
    enc:write_bool(true)
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    local name = dec:read_string()
    local age = dec:read_int32()
    local active = dec:read_bool()
    current_user.age = age

    return {name = name, age = age, active = active}
end

local json_encode do
    function json_encode()
        return string.format('{"name":"%s","age":%d,"active":%s}',
            "Alice Johnson", 30, "true")
    end
end

local json_decode do
    function json_decode()
        local json = '{"name":"Alice Johnson","age":30,"active":true}'
        local age = tonumber(string.match(json, '"age":(%d+)'))
        current_user.age = age
        return {name = "Alice Johnson", age = age, active = true}
    end
end

print("===========================================")
print("XPB V2 Lua Benchmark (Simple Message)")
print("===========================================")
print("Iterations: " .. ITERATIONS .. "\n")

local has_cjson, cjson = pcall(require, "cjson")
if has_cjson then
    print("JSON library: cjson (found)")
    json_encode = function()
        return cjson.encode({name = "Alice Johnson", age = 30, active = true})
    end
    json_decode = function()
        local data = cjson.decode('{"name":"Alice Johnson","age":30,"active":true}')
        current_user.age = data.age
        return data
    end
else
    print("JSON library: cjson (NOT FOUND)")
    print("Install with: apt-get install lua-cjson or luarocks install lua-cjson")
    print("Using string operations as placeholder (NOT FAIR COMPARISON)\n")

    json_encode = function()
        return string.format('{"name":"%s","age":%d,"active":%s}',
            "Alice Johnson", 30, "true")
    end

    json_decode = function()
        local json = '{"name":"Alice Johnson","age":30,"active":true}'
        local age = tonumber(string.match(json, '"age":(%d+)'))
        current_user.age = age
        return {name = "Alice Johnson", age = age, active = true}
    end
end
print("")

local xpb_enc_warm = time_ms(xpb_encode)
local xpb_dec_warm = time_ms(xpb_decode)
local json_enc_warm = time_ms(json_encode)
local json_dec_warm = time_ms(json_decode)

print("Warmup times (ms per operation):")
print(string.format("  XPB   encode: %.3f us", xpb_enc_warm * 1000))
print(string.format("  XPB   decode: %.3f us", xpb_dec_warm * 1000))
print(string.format("  JSON  encode: %.3f us", json_enc_warm * 1000))
print(string.format("  JSON  decode: %.3f us\n", json_dec_warm * 1000))

local xpb_enc = benchmark(xpb_encode)
local xpb_dec = benchmark(xpb_decode)
local json_enc = benchmark(json_encode)
local json_dec = benchmark(json_decode)

print("Benchmark results (ns per operation):")
print(string.format("  XPB   encode: %.0f ns/op", xpb_enc))
print(string.format("  XPB   decode: %.0f ns/op", xpb_dec))
print(string.format("  JSON  encode: %.0f ns/op", json_enc))
print(string.format("  JSON  decode: %.0f ns/op\n", json_dec))

if has_cjson then
    print("Speedup vs JSON:")
    print(string.format("  XPB encode: %.2fx faster", json_enc / xpb_enc))
    print(string.format("  XPB decode: %.2fx faster", json_dec / xpb_dec))
end

print("\n===========================================")
print("Test passed: benchmark executed successfully")
print("===========================================")
