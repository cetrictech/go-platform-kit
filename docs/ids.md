# UUID helpers

The root `id` package owns UUID generation, parsing, validation, and common predicates so consuming services do not need to import the UUID driver directly.

```go
value, err := id.NewV7()
if err != nil {
    return err
}
if id.IsNil(value) || !id.IsV7(value) {
    return errors.New("invalid identifier")
}
```

Use `id.NewV4` for random UUIDs, `id.NewV7` for time-ordered identifiers, `id.Parse` for text, and `id.Validate` for input validation. `id.UUID` remains an alias of the underlying UUID type, so existing database and JSON integrations remain source-compatible.
