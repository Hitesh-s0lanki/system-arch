// This example embeds the limiter without starting a rate-limiter server.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/sdk"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

func main() {
	client, err := sdk.NewMemory([]sdk.Policy{{
		Policy:      policy.Policy{ID: "orders", Identity: identity.User, PerRoute: true},
		FixedWindow: fixedwindow.Config{Limit: 2, Window: time.Minute, MaxKeys: 10000},
	}})
	if err != nil {
		log.Fatal(err)
	}
	// In an application, derive these identifiers from verified authentication
	// and the router's canonical pattern, rather than caller-supplied headers.
	request := service.Request{TenantID: "demo", UserID: "user-42", Route: "POST /orders"}
	for attempt := 1; attempt <= 3; attempt++ {
		decision, err := client.Check(context.Background(), "orders", request)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("attempt=%d allowed=%t remaining=%d reason=%s retry_after=%s\n",
			attempt, decision.Allowed, decision.Remaining, decision.Reason, decision.RetryAfter)
		if !decision.Allowed {
			continue
		}
		// Perform the application operation here. Admitted attempts consume quota
		// even if the operation later fails; there is no refund operation.
	}
}
