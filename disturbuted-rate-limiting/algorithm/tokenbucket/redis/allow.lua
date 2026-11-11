-- One atomic refill/check/consume operation. State and expiry index share a slot.
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
    -- Use the latest full-refill deadline, so a busy identity cannot expire
    -- another identity's partially empty bucket early.
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
local tokens, fraction = capacity, 0
if encoded then
    local state = cjson.decode(encoded)
    tokens, fraction = state[1], state[2]
    local elapsed = now - state[3]
    local windows = math.floor(elapsed / window)
    local missing = capacity - tokens
    if windows >= math.ceil(missing / limit) then
        tokens, fraction = capacity, 0
    else
        local credit = (elapsed % window) * limit + fraction
        tokens = tokens + windows * limit + math.floor(credit / window)
        fraction = credit % window
        if tokens >= capacity then tokens, fraction = capacity, 0 end
    end
end
-- Split the product so every intermediate stays within exact integer precision.
local function refillDelay(missing)
    return math.floor(missing / limit) * window + math.ceil(((missing % limit) * window - fraction) / limit)
end
local result
if tokens >= 1 then
    tokens = tokens - 1
    result = {1, tokens, 0}
else
    result = {0, 0, math.max(1, refillDelay(1))}
end
redis.call('HSET', KEYS[1], field, '[' .. string.format('%.0f', tokens) .. ',' .. string.format('%.0f', fraction) .. ',' .. string.format('%.0f', now) .. ']')
redis.call('ZADD', KEYS[2], now + math.max(1, refillDelay(capacity - tokens)), field)
expireState()
return result
