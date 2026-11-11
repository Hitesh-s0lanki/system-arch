-- All keys share one policy hash slot. Logs are JSON timestamp arrays in a hash;
-- the sorted set indexes each identity's final timestamp expiry for capacity GC.
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
local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now)
for _, field in ipairs(expired) do
    redis.call('HDEL', KEYS[1], field)
    redis.call('ZREM', KEYS[2], field)
end
redis.call('HSET', KEYS[1], '_last', now)
redis.call('PEXPIREAT', KEYS[1], now + window)
if ARGV[1] == 'usage' then return {redis.call('ZCARD', KEYS[2]), 0, 0} end
local field = 'k:' .. ARGV[2]
local encoded = redis.call('HGET', KEYS[1], field)
if not encoded and redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[6]) then
    return {-1, 0, 0}
end
local log = {}
if encoded then
    for _, stamp in ipairs(cjson.decode(encoded)) do
        if stamp > now - window then table.insert(log, stamp) end
    end
end
local limit = tonumber(ARGV[4])
if #log >= limit then return {0, 0, log[1] + window - now} end
table.insert(log, now)
redis.call('HSET', KEYS[1], field, cjson.encode(log))
redis.call('ZADD', KEYS[2], now + window, field)
redis.call('PEXPIREAT', KEYS[2], now + window)
return {1, limit - #log, 0}
