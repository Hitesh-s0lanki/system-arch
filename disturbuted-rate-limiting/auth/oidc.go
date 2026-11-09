package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Hitesh-s0lanki/system-arch/distributed-rate-limiting/identity"
	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCConfig accepts ID tokens from one issuer and audience. Access tokens must
// be validated by their resource server; they are not interchangeable ID tokens.
type OIDCConfig struct {
	Issuer      string `json:"issuer"`
	Audience    string `json:"audience"`
	TenantClaim string `json:"tenant_claim"`
	UserClaim   string `json:"user_claim"`
	APIKeyClaim string `json:"api_key_claim"`
}

type OIDC struct {
	verifier *oidc.IDTokenVerifier
	config   OIDCConfig
}

// NewOIDC discovers signing keys over HTTPS. The verifier handles key rotation,
// signatures, issuer, audience, and expiry. Required identity claims are strings.
func NewOIDC(ctx context.Context, config OIDCConfig) (*OIDC, error) {
	return NewOIDCWithClient(ctx, config, &http.Client{Timeout: 5 * time.Second})
}

// NewOIDCWithClient allows applications to supply a private CA trust store.
// The supplied transport must validate TLS certificates. Request timeouts and
// HTTPS-only redirects are enforced on a copied client.
func NewOIDCWithClient(ctx context.Context, config OIDCConfig, supplied *http.Client) (*OIDC, error) {
	u, err := url.Parse(config.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || config.Audience == "" || config.TenantClaim == "" {
		return nil, fmt.Errorf("auth: OIDC requires HTTPS issuer, audience, and tenant claim")
	}
	if config.UserClaim == "" {
		config.UserClaim = "sub"
	}
	if supplied == nil {
		return nil, fmt.Errorf("auth: HTTP client required")
	}
	client := *supplied
	client.Timeout = 5 * time.Second
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = httpsTransport{transport}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 5 {
			return fmt.Errorf("auth: invalid OIDC redirect")
		}
		return nil
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, &client), config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: OIDC discovery failed")
	}
	return &OIDC{verifier: provider.Verifier(&oidc.Config{ClientID: config.Audience, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}}), config: config}, nil
}

func (o *OIDC) Authenticate(ctx context.Context, token string) (Principal, error) {
	verified, err := o.verifier.Verify(ctx, token)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	var claims map[string]json.RawMessage
	if err := verified.Claims(&claims); err != nil {
		return Principal{}, ErrUnauthenticated
	}
	get := func(name string) string { var value string; _ = json.Unmarshal(claims[name], &value); return value }
	p := Principal{TenantID: get(o.config.TenantClaim), UserID: get(o.config.UserClaim), APIKeyID: get(o.config.APIKeyClaim)}
	if p.UserID == "" && p.APIKeyID == "" {
		return Principal{}, ErrUnauthenticated
	}
	for kind, value := range map[identity.Kind]string{identity.User: p.UserID, identity.APIKey: p.APIKeyID} {
		if value != "" {
			if _, err := identity.Key("oidc", p.TenantID, identity.Subject{Kind: kind, Value: value}, ""); err != nil {
				return Principal{}, ErrUnauthenticated
			}
		}
	}
	return p, nil
}

type httpsTransport struct{ base http.RoundTripper }

func (t httpsTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" {
		return nil, fmt.Errorf("auth: OIDC endpoints require HTTPS")
	}
	return t.base.RoundTrip(r)
}
