# Embedded Go SDK

Import `github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk`
from this Go module. Create one client at application startup and reuse it across
requests. No separate rate-limiter server is needed.

## In-process storage

```go
client, err := sdk.NewMemory([]sdk.Policy{{
    Policy: policy.Policy{
        ID: "orders", Identity: identity.User, PerRoute: true,
    },
    FixedWindow: fixedwindow.Config{
        Limit: 100, Window: time.Minute, MaxKeys: 10000,
    },
}})
if err != nil {
    return err
}

decision, err := client.Check(ctx, "orders", service.Request{
    TenantID: verifiedTenantID,
    UserID: verifiedUserID,
    Route: "POST /orders",
})
if err != nil {
    return err // Validation, cancellation, or backend failure.
}
if !decision.Allowed {
    // Inspect decision.Reason. RetryAfter is a hint for rate_limited only.
    return handleRejection(decision)
}
return createOrder(ctx)
```

The snippet uses the module's `algorithm/fixedwindow`, `identity`, `policy`,
`sdk`, and `service` packages, plus the standard `time` package. A complete
runnable example is available with `go run ./examples/sdk`.

Each policy gets its own rate and `MaxKeys` budget. Tenant, subject, policy, and
optional canonical route separate counters. The capacity budget is shared across
tenants within a policy. Use `identity.IP`, `identity.User`, or `identity.APIKey`;
credential identities are verified IDs, never raw secrets. Policy selection and
tenant identifiers must come from trusted application code.

In-process quota is local to each client instance and resets on restart. Fixed
windows align to clock boundaries and can admit up to twice the configured limit
around a boundary. Allowed checks consume quota even if the subsequent operation
fails. Checking multiple policies sequentially is not atomic.

## Shared Redis storage

Use the same policy settings, namespace, revision, and identity grouping on every
application replica. Supply a caller-owned `go-redis/v9` client:

```go
backend := redis.NewClient(&redis.Options{
    Addr: "127.0.0.1:6379",
    MaxRetries: -1,
    DialTimeout: time.Second,
    ReadTimeout: time.Second,
    WriteTimeout: time.Second,
    ContextTimeoutEnabled: true,
})
defer backend.Close() // Close after active application requests finish.

client, err := sdk.NewRedis(backend, sdk.RedisConfig{
    Policies: policies, // []sdk.Policy, configured as in the RAM example.
    Namespace: "orders-service",
    Revision: "v1",
    Timeout: 250 * time.Millisecond,
})
if err != nil {
    return err
}
if err := client.Ready(ctx); err != nil {
    return err
}
// Use client.Check exactly as in the in-process example.
```

Construction validates settings without connecting. `Ready` verifies write access
and policy compatibility without consuming quota; its Redis configuration guards
persist. Changing a policy's limit, capacity, window, or identity grouping in the
same scope produces errors. Use a new revision for intentional changes and a
coordinated cutover: a new revision creates fresh quota. Redis windows must use
whole milliseconds and be at least one millisecond.

Redis owns time and expiry. Backend errors propagate; the SDK does not retry
ambiguous decisions, switch to RAM, or admit requests on failure. The application
decides how to handle errors. See [production operation](production.md) for Redis
durability and deployment constraints.

## HTTP integration

The client implements `httpmiddleware.Checker`. Pass it to the existing middleware
with the same `policy.Policy` used at construction:

```go
handler, err := httpmiddleware.Wrap(client, httpmiddleware.Config{
    Policy: orderPolicy,
    Route: "POST /orders",
    Authenticator: verifiedBearerAuthenticator,
}, createOrderHandler)
```

Import `middleware/http` as `httpmiddleware`. Authenticated user and credential
policies require an `Authenticator`; IP policies require `AnonymousTenant` and
optionally trusted proxy prefixes. See the [main guide](../README.md) for identity
resolution and status codes, including 429 for exhausted quota and 503 for
capacity or backend failures.

## Lifecycle

Clients start no workers and require no `Close` method. RAM replaces expired
windows during checks. For idle applications that need prompt memory reclamation,
call `client.CleanupExpired()` periodically, or run it through
`maintenance.RunCleanup` in a goroutine whose cancellation and shutdown you own.
`CleanupExpired` returns the number of removed identities and does nothing for
Redis clients. Always shut down application requests before closing Redis.

## Sliding-window logs

Use `sdk.NewSlidingWindowMemory` with `Policy.SlidingWindow` to enforce an exact
rolling interval. For shared Redis state, use `sdk.NewSlidingWindowRedis` with
the same `RedisConfig` structure and `Policy.SlidingWindow`. Existing `NewMemory`
and `NewRedis` constructors continue to use `Policy.FixedWindow`.

See [the sliding-window guide](sliding-window.md) for complete configuration,
storage details, and the RAM/Redis integration test command.

## Sliding-window counters

Use `sdk.NewSlidingCounterMemory` or `sdk.NewSlidingCounterRedis` with
`Policy.SlidingCounter` to estimate rolling usage using two weighted counters.
See [the counter guide](sliding-window-counter.md) for configuration, precision,
retry semantics, and tests. The existing SlidingWindow constructors retain
exact timestamp-log behavior.
