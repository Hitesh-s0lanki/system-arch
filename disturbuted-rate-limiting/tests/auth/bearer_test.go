package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
)

func TestBearer(t *testing.T) {
	token := strings.Repeat("a", 32)
	principal := auth.Principal{TenantID: "tenant", UserID: "42", APIKeyID: "credential-7"}
	credentials := []auth.Credential{{Token: token, Principal: principal}}
	b, err := auth.NewBearer(credentials)
	if err != nil {
		t.Fatal(err)
	}
	credentials[0].UserID = "changed"
	got, err := b.Authenticate(context.Background(), token)
	if err != nil || got != principal {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, token := range []string{"", "short", strings.Repeat("b", 32)} {
		if _, err := b.Authenticate(context.Background(), token); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("got %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Authenticate(ctx, token); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestBearerConfigValidation(t *testing.T) {
	valid := auth.Credential{Token: strings.Repeat("a", 32), Principal: auth.Principal{TenantID: "t", UserID: "u"}}
	for _, entries := range [][]auth.Credential{
		{valid, valid},
		{{Token: "short", Principal: valid.Principal}},
		{{Token: strings.Repeat("a", 32) + " ", Principal: valid.Principal}},
		{{Token: valid.Token, Principal: auth.Principal{TenantID: "t"}}},
		{{Token: valid.Token, Principal: auth.Principal{UserID: "u"}}},
	} {
		if _, err := auth.NewBearer(entries); err == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if _, err := auth.NewBearer(nil); err != nil {
		t.Fatal(err)
	}
}
