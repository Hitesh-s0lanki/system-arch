export type Storage = 'ram' | 'redis'
export type AlgorithmSettings = {
  limit: number
  windowSeconds: number
  maxKeys: number
  capacity: number
  storage: Storage
}
export const defaultSettings: AlgorithmSettings = {
  limit: 100,
  windowSeconds: 60,
  maxKeys: 10000,
  capacity: 20,
  storage: 'ram',
}
const implementations: Record<
  string,
  {
    algorithm: string
    pkg: string
    field: string
    memory: string
    redis: string
    bucket?: boolean
  }
> = {
  'fixed-window': {
    algorithm: 'fixed_window',
    pkg: 'fixedwindow',
    field: 'FixedWindow',
    memory: 'NewMemory',
    redis: 'NewRedis',
  },
  'sliding-window-log': {
    algorithm: 'sliding_window_log',
    pkg: 'slidingwindow',
    field: 'SlidingWindow',
    memory: 'NewSlidingWindowMemory',
    redis: 'NewSlidingWindowRedis',
  },
  'sliding-window-counter': {
    algorithm: 'sliding_window_counter',
    pkg: 'slidingcounter',
    field: 'SlidingCounter',
    memory: 'NewSlidingCounterMemory',
    redis: 'NewSlidingCounterRedis',
  },
  'token-bucket': {
    algorithm: 'token_bucket',
    pkg: 'tokenbucket',
    field: 'TokenBucket',
    memory: 'NewTokenBucketMemory',
    redis: 'NewTokenBucketRedis',
    bucket: true,
  },
  'leaky-bucket': {
    algorithm: 'leaky_bucket',
    pkg: 'leakybucket',
    field: 'LeakyBucket',
    memory: 'NewLeakyBucketMemory',
    redis: 'NewLeakyBucketRedis',
    bucket: true,
  },
}
export function isBucket(id: string) {
  return !!implementations[id]?.bucket
}
export function settingsError(s: AlgorithmSettings, id: string): string | null {
  if (!Number.isInteger(s.limit) || s.limit < 1 || s.limit > 1000000)
    return 'Limit must be a whole number from 1 to 1,000,000.'
  if (
    !Number.isInteger(s.windowSeconds) ||
    s.windowSeconds < 1 ||
    s.windowSeconds > 3600
  )
    return 'Window must be a whole number from 1 to 3,600 seconds.'
  if (!Number.isInteger(s.maxKeys) || s.maxKeys < 1 || s.maxKeys > 1000000)
    return 'Maximum identities must be a whole number from 1 to 1,000,000.'
  if (
    isBucket(id) &&
    (!Number.isInteger(s.capacity) || s.capacity < 1 || s.capacity > 1000000)
  )
    return 'Capacity must be a whole number from 1 to 1,000,000.'
  return null
}
export function algorithmExamples(id: string, settings: AlgorithmSettings) {
  const impl = implementations[id]
  if (!impl) throw new Error('Unsupported algorithm')
  const error = settingsError(settings, id)
  if (error) throw new Error(error)
  const { limit, windowSeconds, maxKeys, capacity, storage } = settings
  const shared = storage === 'redis'
  const revision = `v1-${limit}-${windowSeconds}-${maxKeys}${impl.bucket ? `-${capacity}` : ''}`
  const config =
    JSON.stringify(
      {
        version: 1,
        listen: ':8080',
        upstream: 'http://backend:8080',
        storage: {
          mode: storage,
          namespace: 'proxy-demo',
          timeout: '1s',
          pool_size: 32,
          ...(shared ? { redis_url: 'redis://redis:6379/0' } : {}),
        },
        cleanup_interval: '1s',
        upstream_timeout: '10s',
        anonymous_tenant: 'public',
        trusted_proxies: [],
        forward_authorization: false,
        routes: [
          {
            pattern: '/',
            algorithm: impl.algorithm,
            policy: { id: 'demo-ip', identity: 'ip', per_route: false },
            limit,
            window: `${windowSeconds}s`,
            max_keys: maxKeys,
            ...(impl.bucket ? { capacity } : {}),
            revision,
          },
        ],
        max_body_bytes: 1048576,
        max_concurrent: 128,
      },
      null,
      2,
    ) + '\n'
  const compose = `name: algorithm-demo

x-runtime: &runtime
  image: rate-limiter:local
  build:
    context: .
    target: runtime
  read_only: true
  cap_drop: [ALL]
  security_opt: [no-new-privileges:true]
  stop_grace_period: 10s
  mem_limit: 512m
  cpus: 1
  pids_limit: 128

services:
  gateway:
    <<: *runtime
    ports:
      - "127.0.0.1:8088:8080"
    volumes:
      - ./proxy.algorithm.json:/etc/ratelimit/proxy.json:ro
    depends_on:
      backend:
        condition: service_healthy${shared ? '\n      redis:\n        condition: service_healthy' : ''}

  backend:
    <<: *runtime
    entrypoint: ["/demo-backend"]
    command: []
    healthcheck:
      test: [CMD, /ratelimitd, -healthcheck, "http://127.0.0.1:8080/healthz"]
      interval: 2s
      timeout: 2s
      retries: 15
${
  shared
    ? `
  redis:
    image: redis:7.4.7-alpine
    ports:
      - "127.0.0.1:6380:6379"
    command: [redis-server, --appendonly, "yes", --maxmemory, 128mb, --maxmemory-policy, noeviction]
    volumes:
      - algorithm-redis:/data
    healthcheck:
      test: [CMD, redis-cli, ping]
      interval: 2s
      timeout: 2s
      retries: 15

volumes:
  algorithm-redis:
`
    : ''
}`
  const modulePath =
    'github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting'
  const go = `package main

import (
    "context"
    "fmt"
    "log"
    "time"${shared ? '\n    "os"' : ''}

    "${modulePath}/algorithm/${impl.pkg}"
    "${modulePath}/identity"
    "${modulePath}/policy"
    "${modulePath}/sdk"
    "${modulePath}/service"${shared ? '\n    redis "github.com/redis/go-redis/v9"' : ''}
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    policies := []sdk.Policy{{
        Policy: policy.Policy{ID: "demo-ip", Identity: identity.IP},
        ${impl.field}: ${impl.pkg}.Config{
            Limit: ${limit}, Window: ${windowSeconds}*time.Second, MaxKeys: ${maxKeys},${impl.bucket ? `\n            Capacity: ${capacity},` : ''}
        },
    }}
${
  shared
    ? `
    redisURL := os.Getenv("REDIS_URL")
    if redisURL == "" {
        redisURL = "redis://127.0.0.1:6380/0" // Local Compose Redis.
    }
    options, err := redis.ParseURL(redisURL)
    if err != nil { log.Fatal(err) }
    options.MaxRetries = -1
    options.ContextTimeoutEnabled = true
    options.DialTimeout = time.Second
    options.ReadTimeout = time.Second
    options.WriteTimeout = time.Second
    backend := redis.NewClient(options)
    defer backend.Close()
    client, err := sdk.${impl.redis}(backend, sdk.RedisConfig{
        Policies: policies, Namespace: "sdk-demo", Revision: "${revision}", Timeout: time.Second,
    })`
    : `    client, err := sdk.${impl.memory}(policies)`
}
    if err != nil { log.Fatal(err) }
    if err := client.Ready(ctx); err != nil { log.Fatal(err) }

    // Resolve identities through trusted application code.
    decision, err := client.Check(ctx, "demo-ip", service.Request{
        TenantID: "public", IP: "203.0.113.10",
    })
    if err != nil { log.Fatal(err) }
    fmt.Printf("allowed=%t remaining=%d retry_after=%s\\n",
        decision.Allowed, decision.Remaining, decision.RetryAfter)
    if !decision.Allowed { return }
    // Call your application handler here.
}
`
  const commands = `# Run from disturbuted-rate-limiting/ after saving both files.
docker compose -f compose.algorithm.yaml up -d --build --force-recreate --wait
curl -i http://127.0.0.1:8088/orders

# Stop the demo; Redis data is retained.
docker compose -f compose.algorithm.yaml down`
  return { config, compose, go, commands }
}
