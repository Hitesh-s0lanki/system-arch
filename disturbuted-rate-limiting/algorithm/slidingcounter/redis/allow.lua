-- Two integer counters per identity; all keys share a policy hash slot.
local signature = ARGV[5]
redis.call('SET', KEYS[3], signature, 'NX')
if redis.call('GET', KEYS[3]) ~= signature then
    return redis.error_reply('policy configuration mismatch')
end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local last = tonumber(redis.call('HGET', KEYS[1], '_last') or '0')
if now < last then now = last end
local window = tonumber(ARGV[3])
for _, field in ipairs(redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now)) do
    redis.call('HDEL', KEYS[1], field)
    redis.call('ZREM', KEYS[2], field)
end
redis.call('HSET', KEYS[1], '_last', now)
-- Keep the hash at least as long as every counter's contribution.
redis.call('PEXPIREAT', KEYS[1], now + 2 * window)
if ARGV[1] == 'usage' then return {redis.call('ZCARD', KEYS[2]), 0, 0} end
local field = 'k:' .. ARGV[2]
local encoded = redis.call('HGET', KEYS[1], field)
if not encoded and redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[6]) then return {-1, 0, 0} end
local start = now - now % window
local b = {start, 0, 0}
if encoded then b = cjson.decode(encoded) end
if start > b[1] then
    b[2] = (start - b[1] == window) and b[3] or 0
    b[3], b[1] = 0, start
end
local elapsed = now - start
local limit = tonumber(ARGV[4])
local used = b[3] * window + b[2] * (window - elapsed)
if used + window > limit * window then
    local retry
    if b[3] >= limit then
        retry = window - elapsed + math.ceil(window / b[3])
    else
        retry = math.ceil((used - (limit - 1) * window) / b[2])
    end
    return {0, 0, retry}
end
b[3] = b[3] + 1
redis.call('HSET', KEYS[1], field, cjson.encode(b))
redis.call('ZADD', KEYS[2], start + 2 * window, field)
redis.call('PEXPIREAT', KEYS[2], now + 2 * window)
return {1, math.floor((limit * window - used - window) / window), 0}
