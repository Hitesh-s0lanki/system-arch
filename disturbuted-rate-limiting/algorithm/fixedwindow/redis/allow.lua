-- One policy hash: three metadata fields plus at most MaxKeys counters.
-- Check, increment, capacity enforcement, and expiration are one atomic command.
local field = 'k:' .. ARGV[1]
local limit = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local capacity = tonumber(ARGV[4])
local signature = ARGV[5]
local config = redis.call('HGET', KEYS[1], '_config')
if config and config ~= signature then
    return redis.error_reply('policy configuration mismatch')
end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local last = tonumber(redis.call('HGET', KEYS[1], '_last') or '0')
if now < last then now = last end
local start = math.floor(now / window) * window
local previous = tonumber(redis.call('HGET', KEYS[1], '_window') or '-1')
if previous ~= start then
    redis.call('DEL', KEYS[1])
end
redis.call('HSET', KEYS[1], '_window', start, '_last', now, '_config', signature)
redis.call('PEXPIREAT', KEYS[1], start + window)
local count = tonumber(redis.call('HGET', KEYS[1], field) or '0')
if count == 0 and redis.call('HLEN', KEYS[1]) - 3 >= capacity then
    return {-1, 0, 0}
end
if count >= limit then
    return {0, 0, start + window - now}
end
count = count + 1
redis.call('HSET', KEYS[1], field, count)
return {1, limit - count, 0}
