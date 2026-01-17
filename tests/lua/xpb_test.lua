-- XPB V2 Lua Runtime Tests
-- Tests round-trip encoding/decoding for all types

local xpb = require "xpb"

local tests_passed = 0
local tests_failed = 0

local function test(name, cond)
    if cond then
        print(string.format("  [PASS] %s", name))
        tests_passed = tests_passed + 1
    else
        print(string.format("  [FAIL] %s", name))
        tests_failed = tests_failed + 1
    end
end

-- Test bool round-trip
local function test_bool()
    print("\n=== Test Bool ===")

    local enc = xpb.Encoder(64)
    enc:write_bool(true)
    enc:write_bool(false)
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    test("bool true", dec:read_bool() == true)
    test("bool false", dec:read_bool() == false)
end

-- Test int32 round-trip
local function test_int32()
    print("\n=== Test Int32 ===")

    local values = {0, 1, -1, 100, -100, 2147483647, -2147483648}
    local enc = xpb.Encoder(256)
    for i, v in ipairs(values) do
        enc:write_int32(v)
    end
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    for i, v in ipairs(values) do
        test(string.format("int32 value %d", v), dec:read_int32() == v)
    end
end

-- Test int64 round-trip
local function test_int64()
    print("\n=== Test Int64 ===")

    local values = {0, 1, -1, 1000000000, -1000000000, 9223372036854775807, -9223372036854775807}
    local enc = xpb.Encoder(256)
    for i, v in ipairs(values) do
        enc:write_int64(v)
    end
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    for i, v in ipairs(values) do
        test(string.format("int64 value %d", v), dec:read_int64() == v)
    end
end

-- Test float32 round-trip
local function test_float32()
    print("\n=== Test Float32 ===")

    local values = {0.0, 1.0, -1.0, 3.14159, -273.15}
    local enc = xpb.Encoder(256)
    for i, v in ipairs(values) do
        enc:write_float32(v)
    end
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    for i, v in ipairs(values) do
        local decoded = dec:read_float32()
        test(string.format("float32 value %f", v), math.abs(decoded - v) < 0.0001)
    end
end

-- Test float64 round-trip
local function test_float64()
    print("\n=== Test Float64 ===")

    local values = {0.0, 1.0, -1.0, 3.14159265358979, -273.15, 1e100}
    local enc = xpb.Encoder(256)
    for i, v in ipairs(values) do
        enc:write_float64(v)
    end
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    for i, v in ipairs(values) do
        local decoded = dec:read_float64()
        test(string.format("float64 value %g", v), math.abs(decoded - v) < 1e-10)
    end
end

-- Test string round-trip
local function test_string()
    print("\n=== Test String ===")

    local values = {"", "a", "hello", "hello world", "1234567890", "This is a longer string with many characters"}
    for i, v in ipairs(values) do
        local enc = xpb.Encoder(256)
        enc:write_string(v)
        local data = enc:finish()

        local dec = xpb.Decoder(data)
        local decoded = dec:read_string()
        test(string.format("string '%s'", v), decoded == v)
    end

    -- Test long string (>254 chars)
    local long_str = string.rep("x", 300)
    local enc = xpb.Encoder(512)
    enc:write_string(long_str)
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    local decoded = dec:read_string()
    test("long string (>254 chars) length", #decoded == 300)
    test("long string content", decoded == long_str)
end

-- Test bytes round-trip
local function test_bytes()
    print("\n=== Test Bytes ===")

    local data1 = string.char(0x01, 0x02, 0x03, 0x04, 0x05)

    -- Generate 256 bytes
    local data2 = ""
    for i = 0, 255 do
        data2 = data2 .. string.char(i)
    end

    local enc = xpb.Encoder(512)
    enc:write_bytes(data1)
    enc:write_bytes(data2)
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    local decoded1 = dec:read_bytes()
    local decoded2 = dec:read_bytes()

    test("small bytes length", #decoded1 == #data1)
    test("small bytes content", decoded1 == data1)
    test("large bytes length", #decoded2 == #data2)
    test("large bytes content", decoded2 == data2)
end

-- Test nested messages
local function test_nested_message()
    print("\n=== Test Nested Message ===")

    -- Encode inner message
    local inner_enc = xpb.Encoder(64)
    inner_enc:write_string("inner_value")
    inner_enc:write_int32(42)
    local inner_data = inner_enc:finish()

    -- Encode outer message
    local outer_enc = xpb.Encoder(256)
    outer_enc:write_string("outer_name")
    outer_enc:write_message(inner_data)
    local outer_data = outer_enc:finish()

    -- Decode
    local dec = xpb.Decoder(outer_data)
    local name = dec:read_string()
    local inner_out = dec:read_message_bytes()

    test("outer string", name == "outer_name")
    test("inner message length", #inner_out == #inner_data)

    local inner_dec = xpb.Decoder(inner_out)
    local inner_str = inner_dec:read_string()
    local inner_int = inner_dec:read_int32()

    test("inner string", inner_str == "inner_value")
    test("inner int", inner_int == 42)
end

-- Test all types combined
local function test_all_types()
    print("\n=== Test All Types Combined ===")

    local enc = xpb.Encoder(256)
    enc:write_bool(true)
    enc:write_int32(-12345)
    enc:write_int64(9876543210)
    enc:write_float32(3.14)
    enc:write_float64(2.718281828)
    enc:write_string("test string")
    enc:write_bytes(string.char(0xDE, 0xAD, 0xBE, 0xEF))
    local data = enc:finish()

    local dec = xpb.Decoder(data)
    test("bool", dec:read_bool() == true)
    test("int32", dec:read_int32() == -12345)
    test("int64", dec:read_int64() == 9876543210)
    test("float32", math.abs(dec:read_float32() - 3.14) < 0.001)
    test("float64", math.abs(dec:read_float64() - 2.718281828) < 1e-9)
    test("string", dec:read_string() == "test string")
    local bytes = dec:read_bytes()
    test("bytes length", #bytes == 4)
    test("bytes content", bytes == string.char(0xDE, 0xAD, 0xBE, 0xEF))
end

-- Run all tests
print("===========================================")
print("XPB V2 Lua Runtime Tests")
print("===========================================")

test_bool()
test_int32()
test_int64()
test_float32()
test_float64()
test_string()
test_bytes()
test_nested_message()
test_all_types()

print("\n===========================================")
print(string.format("Results: %d passed, %d failed", tests_passed, tests_failed))
print("===========================================")

if tests_failed > 0 then
    os.exit(1)
end
