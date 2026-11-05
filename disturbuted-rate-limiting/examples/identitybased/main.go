// This example uses trusted sample identities, not HTTP authentication.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/algorithm/fixedwindow"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/policy"
	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/service"
)

func main() {
	var bindings []service.Binding
	for _, p := range []policy.Policy{
		{ID: "per-ip", Identity: identity.IP},
		{ID: "per-user", Identity: identity.User},
		{ID: "per-credential", Identity: identity.APIKey},
		{ID: "per-user-route", Identity: identity.User, PerRoute: true},
	} {
		limiter, err := fixedwindow.New(fixedwindow.Config{
			Limit: 2, Window: time.Minute, MaxKeys: 10000,
		})
		if err != nil {
			log.Fatal(err)
		}
		bindings = append(bindings, service.Binding{Policy: p, Limiter: limiter})
	}
	s, err := service.New(bindings)
	if err != nil {
		log.Fatal(err)
	}
	request := service.Request{
		TenantID: "demo", IP: "192.0.2.1", UserID: "user-42",
		APIKeyID: "credential-7", Route: "GET /orders/{id}",
	}
	for _, policyID := range []string{"per-ip", "per-user", "per-credential", "per-user-route"} {
		for i := 1; i <= 3; i++ {
			decision, err := s.Check(context.Background(), policyID, request)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("%s request=%d allowed=%t remaining=%d reason=%s\n",
				policyID, i, decision.Allowed, decision.Remaining, decision.Reason)
		}
	}
}
