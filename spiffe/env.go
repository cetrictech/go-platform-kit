package spiffe

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// FromEnv reads GRPC_SECURITY_MODE, SPIFFE_* and APP_SPIFFE_* variables used by Staypace Cloud services.
// enabled is true when GRPC_SECURITY_MODE=spire or APP_ENABLE_SPIFFE=true. Call New only when enabled.
func FromEnv() (Config, bool, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GRPC_SECURITY_MODE")))
	switch mode {
	case "", "plaintext", "insecure":
	case "spire":
	default:
		return Config{}, false, errors.New("spiffe: GRPC_SECURITY_MODE must be spire, plaintext, or insecure")
	}
	httpMTLS, e := optionalBool("APP_ENABLE_SPIFFE")
	if e != nil {
		return Config{}, false, e
	}
	enabled := mode == "spire" || httpMTLS
	if !enabled {
		return Config{}, false, nil
	}
	c := Config{
		Socket:           firstEnv("SPIRE_AGENT_SOCKET", "SPIFFE_ENDPOINT_SOCKET"),
		TrustDomain:      firstEnv("SPIFFE_TRUST_DOMAIN", "APP_SPIFFE_TRUST_DOMAIN"),
		ExpectedID:       firstEnv("SPIFFE_ID", "APP_SPIFFE_ID"),
		AllowTrustDomain: true,
	}
	if c.Socket == "" {
		c.Socket = defaultSocket
	}
	if strings.HasPrefix(c.Socket, "/") {
		c.Socket = "unix://" + c.Socket
	}
	c.AllowedPeers, e = csvEnv("APP_SPIFFE_ALLOWED_IDS")
	if e != nil {
		return Config{}, false, e
	}
	clients, e := csvEnv("SPIFFE_ALLOWED_CLIENT_IDS")
	if e != nil {
		return Config{}, false, e
	}
	c.AllowedPeers = mergeCSV(c.AllowedPeers, clients)
	if len(c.AllowedPeers) > 0 {
		c.AllowTrustDomain = false
	}
	c.Timeout, e = optionalDuration("APP_SPIFFE_STARTUP_TIMEOUT", 10*time.Second)
	if e != nil {
		return Config{}, false, e
	}
	if c.TrustDomain == "" || c.ExpectedID == "" {
		return Config{}, false, errors.New("spiffe: SPIFFE_TRUST_DOMAIN and SPIFFE_ID are required when SPIRE is enabled")
	}
	if _, _, _, _, e = c.policy(); e != nil {
		return Config{}, false, e
	}
	return c, true, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func csvEnv(key string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	values := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, errors.New("spiffe: " + key + " must not contain empty entries")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values, nil
}

func mergeCSV(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, item := range append(append([]string{}, a...), b...) {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func optionalBool(key string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, nil
	}
	v, e := strconv.ParseBool(raw)
	if e != nil {
		return false, errors.New("spiffe: " + key + " must be a boolean")
	}
	return v, nil
}

func optionalDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, e := time.ParseDuration(raw)
	if e != nil || v <= 0 {
		return 0, errors.New("spiffe: " + key + " must be a positive duration")
	}
	return v, nil
}
