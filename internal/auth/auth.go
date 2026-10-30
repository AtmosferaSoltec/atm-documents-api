package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const roleService = "service"

// Claims are the atm-auth-api access token claims this service reads.
type Claims struct {
	jwt.RegisteredClaims
	Role   string `json:"role"`
	Tenant string `json:"tenant"`
	System string `json:"system"`
}

// Verifier validates atm-auth-api service tokens locally with the public key.
type Verifier struct {
	key     *rsa.PublicKey
	issuer  string
	systems map[string]bool
}

// New builds a Verifier. Only the listed systems may call the service.
func New(publicKeyBase64, issuer string, allowedSystems []string) (*Verifier, error) {
	data, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return nil, errors.New("invalid AUTH_PUBLIC_KEY_BASE64")
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil || key.N.BitLen() < 2048 {
		return nil, errors.New("invalid RSA public key (minimum 2048 bits)")
	}
	systems := make(map[string]bool, len(allowedSystems))
	for _, system := range allowedSystems {
		systems[system] = true
	}
	return &Verifier{key: key, issuer: issuer, systems: systems}, nil
}

// Verify checks signature, issuer, expiry and that the token is a service
// token of an allowed system.
func (v *Verifier) Verify(raw string) (Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(raw, &claims,
		func(*jwt.Token) (any, error) { return v.key, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || claims.Subject == "" || claims.IssuedAt == nil || claims.Role != roleService {
		return Claims{}, errors.New("invalid access token")
	}
	if !v.systems[claims.System] {
		return Claims{}, errors.New("system not allowed")
	}
	return claims, nil
}

// Middleware rejects requests without a valid service token.
func (v *Verifier) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpserver.Error(c, 401, "unauthorized", "Missing or invalid access token.")
			return
		}
		claims, err := v.Verify(parts[1])
		if err != nil {
			httpserver.Error(c, 401, "unauthorized", "Missing or invalid access token.")
			return
		}
		c.Set("auth_claims", claims)
		c.Next()
	}
}

// FromContext returns the verified claims of the current request.
func FromContext(c *gin.Context) Claims {
	value, _ := c.Get("auth_claims")
	claims, _ := value.(Claims)
	return claims
}
