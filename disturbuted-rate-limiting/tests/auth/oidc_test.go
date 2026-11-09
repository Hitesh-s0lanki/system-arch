package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/auth"
	"github.com/go-jose/go-jose/v4"
)

func TestOIDCVerificationAndSigningKeyRotation(t *testing.T) {
	first, _ := rsa.GenerateKey(rand.Reader, 2048)
	second, _ := rsa.GenerateKey(rand.Reader, 2048)
	var mu sync.Mutex
	key := jose.JSONWebKey{Key: &first.PublicKey, KeyID: "first", Algorithm: "RS256", Use: "sig"}
	issuer := ""
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			mu.Lock()
			snapshot := key
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{snapshot}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	verifier, err := auth.NewOIDCWithClient(context.Background(), auth.OIDCConfig{Issuer: issuer, Audience: "gateway", TenantClaim: "tenant"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	sign := func(private *rsa.PrivateKey, kid string, changes map[string]any) string {
		claims := map[string]any{"iss": issuer, "aud": "gateway", "sub": "user", "tenant": "customer", "exp": time.Now().Add(time.Minute).Unix(), "iat": time.Now().Unix()}
		for k, v := range changes {
			claims[k] = v
		}
		raw, _ := json.Marshal(claims)
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: private}, (&jose.SignerOptions{}).WithHeader("kid", kid))
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signer.Sign(raw)
		if err != nil {
			t.Fatal(err)
		}
		token, err := signed.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	good := sign(first, "first", nil)
	p, err := verifier.Authenticate(context.Background(), good)
	if err != nil || p.TenantID != "customer" || p.UserID != "user" {
		t.Fatalf("valid token: %+v %v", p, err)
	}
	for _, changes := range []map[string]any{{"iss": "https://wrong.example"}, {"aud": "other"}, {"exp": time.Now().Add(-time.Hour).Unix()}, {"tenant": 42}, {"tenant": ""}, {"sub": ""}} {
		if _, err := verifier.Authenticate(context.Background(), sign(first, "first", changes)); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Fatalf("invalid claims accepted: %v err=%v", changes, err)
		}
	}
	if _, err := verifier.Authenticate(context.Background(), sign(second, "first", nil)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatal("forged signature accepted")
	}
	mu.Lock()
	key = jose.JSONWebKey{Key: &second.PublicKey, KeyID: "second", Algorithm: "RS256", Use: "sig"}
	mu.Unlock()
	if _, err := verifier.Authenticate(context.Background(), sign(second, "second", nil)); err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
}
