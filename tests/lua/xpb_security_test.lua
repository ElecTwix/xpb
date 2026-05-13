-- XPB V2 Lua Runtime Security Validation (post-hardening)
--
-- Each test exercises a class of input that pre-hardening either succeeded
-- silently (returning garbage) or hung the interpreter. After the runtime
-- gained ensure_bytes() + read_array_count(element_min_bytes, max_elements),
-- every one of these inputs now raises a clear Lua error. The script
-- asserts that error is raised — if a future change removes the guard, a
-- test goes from "errored as expected" back to "succeeded silently" and
-- the script fails.

local xpb = require("xpb")

local passed = 0
local failed = 0

local function expect_error(name, fn)
    local ok, err = pcall(fn)
    if not ok then
        print(string.format("  [PASS] %s (errored: %s)", name, err))
        passed = passed + 1
    else
        print(string.format("  [FAIL] %s — call succeeded; expected an error", name))
        failed = failed + 1
    end
end

print("===========================================")
print("XPB V2 Lua Runtime Security Validation")
print("===========================================")

-- XPB-109: read_bool over a 0-byte buffer must raise, not return true.
print("\n=== XPB-109: read_bool past EOF must error ===")
expect_error("read_bool over empty buffer", function()
    local dec = xpb.Decoder("")
    return dec:read_bool()
end)

-- XPB-109: read_int32 over a too-short buffer must raise.
print("\n=== XPB-109: read_int32 past EOF must error ===")
expect_error("read_int32 over 3-byte buffer", function()
    local dec = xpb.Decoder("\x01\x02\x03")
    return dec:read_int32()
end)

-- XPB-108: read_array_int32 requires an explicit max — calling it without
-- the new argument should error.
print("\n=== XPB-108: read_array_int32 requires explicit max ===")
expect_error("read_array_int32 with no max arg", function()
    local enc = xpb.Encoder(8)
    enc:write_int32(1000)
    local dec = xpb.Decoder(enc:finish())
    return dec:read_array_int32()
end)

-- XPB-108: even with a max, an oversized wire count is rejected.
print("\n=== XPB-108: oversized count rejected upfront ===")
expect_error("read_array_int32 with count > max", function()
    local enc = xpb.Encoder(8)
    enc:write_int32(1000) -- 1000 elements claimed
    local dec = xpb.Decoder(enc:finish())
    return dec:read_array_int32(64) -- caller's budget is 64
end)

-- XPB-108: buffer-bound check still works (4-byte buffer can't hold 1000
-- int32 elements).
print("\n=== XPB-108: count exceeding buffer is rejected ===")
expect_error("read_array_int32 with count > buffer", function()
    local enc = xpb.Encoder(8)
    enc:write_int32(1000)
    local dec = xpb.Decoder(enc:finish())
    return dec:read_array_int32(1 << 20) -- huge max, but buffer is too small
end)

-- XPB-109: skip past EOF must error.
print("\n=== XPB-109: skip(n) past EOF must error ===")
expect_error("skip past buffer end", function()
    local dec = xpb.Decoder("ab")
    dec:skip(100000)
end)

-- Sanity: legitimate reads still work.
print("\n=== Regression: legitimate round-trip still works ===")
local function legit_roundtrip()
    local enc = xpb.Encoder(64)
    enc:write_int32(3)
    enc:write_int32(10)
    enc:write_int32(20)
    enc:write_int32(30)
    local dec = xpb.Decoder(enc:finish())
    local arr = dec:read_array_int32(16) -- caller budget = 16
    return arr[1] == 10 and arr[2] == 20 and arr[3] == 30
end
if legit_roundtrip() then
    print("  [PASS] honest count + elements decoded correctly")
    passed = passed + 1
else
    print("  [FAIL] legitimate payload no longer round-trips")
    failed = failed + 1
end

print(string.format("\nResults: %d passed, %d failed", passed, failed))
os.exit(failed > 0 and 1 or 0)
