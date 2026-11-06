# Configuration

```go
var cfg struct {
    Port int `env:"PORT" default:"8080"`
    Token string `env:"TOKEN" required:"true" secret:"true"`
    Database struct { Timeout time.Duration `default:"5s"` }
}
err := config.Load(&cfg, config.Options{Prefix: "APP_"})
```

Nested environment names use underscores: APP_DATABASE_TIMEOUT. Explicit `env` tags replace the field name. `env:"-"` skips loading. Existing nonzero values survive defaults; environment values override them. An explicitly supplied JSON reader is loaded before environment overrides. Unknown JSON fields and trailing documents fail. No `.env` file or implicit process environment mutation is used.

Load supports strings, booleans, signed/unsigned integers, floating point, durations, timestamps, UUIDs, URL values, pointers, comma-separated slices and TextUnmarshaler implementations. Environment and default strings for `time.Time` accept RFC3339 and zone-less UTC values (`2006-01-02T15:04:05Z`, `2006-01-02 15:04:05`, `2006-01-02`). `id.UUID` accepts the textual forms accepted by `id.Parse`. JSON files still use encoding/json, so timestamps there are RFC3339 and UUIDs are strings. Errors identify keys and types without echoing secret values. A temporary value is validated before assignment; ordinary conversion failures leave the destination unchanged. Transactional copying preserves programmatic fields excluded from JSON; do not use cyclic maps/slices as configuration. JSON file inputs must be serializable.

Implement `Validate() error` on the configuration or supply Options.Validate. Use Options.Lookup for programmatic overrides and tests. Getter helpers return fallback values on invalid input; use Load when malformed values must fail startup. Dump recursively masks secret-tagged and sensitive-name struct fields; map keys receive the same sensitive-name filtering. Keep secrets in explicitly tagged or named fields rather than arbitrary free text.

`Options.Overrides` and `Options.Environment` select explicit JSON profile overlays before environment overrides. No profile is selected from process environment implicitly. Defaults have lowest priority, followed by existing programmatic values, JSON input, selected profile, environment lookup and explicit `Options.Values`. `config.Validate` runs validation independently of loading. Recursive copy/dump traversal is bounded; cyclic configuration inputs fail rather than recursing indefinitely.

For small startup settings that do not need a full configuration struct, use the exported environment helpers:

```go
port, err := config.OptionalIntEnv("PORT", 8080, 1, 65535)
topics, err := config.OptionalCSVEnv("KAFKA_CONSUMER_TOPICS")
timeout, err := config.OptionalDurationEnv("APP_TIMEOUT", 5*time.Second)
```

`RequiredStringEnv` reports a missing variable without exposing its value. `OptionalBoolEnv`, `OptionalStringEnv`, and the validators (`ValidateListenAddress`, `ValidatePort`, `ValidateServiceURL`, `ValidateOrigin`) centralize parsing and validation for consuming services.
