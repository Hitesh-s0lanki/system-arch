-- One atomic drain/check/add operation. State and expiry index share a slot.
local signature = ARGV[5]
redis.call('SET', KEYS[3], signature, 'NX')
if redis.call('GET', KEYS[3]) ~= signature then
    return redis.error_reply('policy configuration mismatch')
end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local last = tonumber(redis.call('HGET', KEYS[1], '_last') or '0')
if now < last then now = last end
local window, limit, capacity = tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[7])
local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now)
for _, field in ipairs(expired) do
    redis.call('HDEL', KEYS[1], field)
    redis.call('ZREM', KEYS[2], field)
end
local function expireState()
    -- Use the latest full-drain deadline, so a busy identity cannot expire
    -- another identity's partially full bucket early.
    local latest = redis.call('ZREVRANGE', KEYS[2], 0, 0, 'WITHSCORES')
    if #latest == 0 then
        redis.call('DEL', KEYS[1], KEYS[2])
    else
        local deadline = tonumber(latest[2])
        redis.call('HSET', KEYS[1], '_last', now)
        redis.call('PEXPIREAT', KEYS[1], deadline)
        redis.call('PEXPIREAT', KEYS[2], deadline)
    end
end
if ARGV[1] == 'usage' then
    local count = redis.call('ZCARD', KEYS[2])
    expireState()
    return {count, 0, 0}
end
local field = 'k:' .. ARGV[2]
local encoded = redis.call('HGET', KEYS[1], field)
if not encoded and redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[6]) then
    expireState()
    return {-1, 0, 0}
end
-- Store load in window milliseconds: one request adds window, each elapsed
-- millisecond drains limit. This avoids fractional-request rounding.
local level = 0
if encoded then
    local state = cjson.decode(encoded)
    level = math.max(0, state[1] - (now - state[2]) * limit)
end
local result
if level + window <= capacity * window then
    level = level + window
    result = {1, math.floor(capacity - level / window), 0}
else
    result = {0, 0, math.max(1, math.ceil((level + window - capacity * window) / limit))}
end
-- cjson's default precision is insufficient for large fractional balances.
redis.call('HSET', KEYS[1], field, '[' .. string.format('%.17g', level) .. ',' .. string.format('%.0f', now) .. ']')
redis.call('ZADD', KEYS[2], now + math.max(1, math.ceil(level / limit)), field)
expireState()
return result
