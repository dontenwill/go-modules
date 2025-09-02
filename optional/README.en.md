# Optional[T] – Unified Error & Return Pattern

This document describes the current implementation of the generic `Optional[T]` structure in `go-common/optional.go` and its idiomatic usage in the project.

## Motivation

Instead of repeatedly propagating `(T, error)` tuples or relying on `panic/recover`, `Optional[T]` encapsulates either a value OR an error (plus an optional error code) in a single type and enables:

- Explicit, enforced error handling
- Uniform semantics for error codes
- Simple forwarding / conversion between optional types
- Optional integration with central error handlers

## Core Type

```go
type Optional[T any] struct {
    Value     T        // Contained value (zero value if unset or error)
    Error     error    // Error object (nil if no error)
    ErrorCode uint32   // Error code (0 if no error)
}
```

Additional sentinel / helper types:

```go
type Void struct{}
type Opt[T any] struct{} // Factory used to "fix" the type in early returns
```

Constant:

```go
const PANIC_CODE = math.MaxUint32 // Reserved: triggers panic when raised via CodeErr / Cast
```

## Methods of Optional[T]

| Method                | Purpose                                                                           |
| --------------------- | --------------------------------------------------------------------------------- |
| `IsError() bool`      | True if `Error != nil` or an `ErrorCode != 0` is present                          |
| `HasErrorCode() bool` | True if `ErrorCode != 0`                                                          |
| `IsSome() bool`       | True if `Value` is not the zero value of the type (beware legitimate zero values) |
| `Unwrap() T`          | Returns the value or panics if an error is present                                |
| `String() string`     | Renders value or error message as text                                            |
| `ToGo() (T, error)`   | Bridge back to the classic Go pattern                                             |

Important: Always use `IsError()` to check for errors – not `IsSome()`. A legitimate zero value (e.g. `0`, `""`, `nil` slice) makes `IsSome()` return false even on success.

## Constructor Functions

```go
func Ok[T any](value T) Optional[T]                // Success
func Err[T any](err interface{}) Optional[T]       // Error without code
func CodeErr[T any](code uint32, err interface{}) Optional[T] // Error with code & handler cascade
func Cast[T any, U any](another Optional[U]) Optional[T]       // Forward / possible type conversion
func GoOpt[T any](value T, err error) Optional[T]  // From classic (T,error)
func None[T any]() Optional[T]                     // Empty: neither value nor error

// Factory helper type (no instance required)
func (Opt[T]) Err(err interface{}) Optional[T]
func (Opt[T]) CodeErr(code uint32, err interface{}) Optional[T]
func (Opt[T]) None() Optional[T]
```

### Behavior & Nuances

Compact overview of runtime behavior of helpers and fields:

1. Ok
   - Sets only `Value`; `Error` = nil; `ErrorCode` = 0.
2. Err
   - Wrapper for `CodeErr(0, err)`.
   - If an `errorHandler` is installed, it may consume the error (`code=0, err=nil`) → empty Optional.
   - Otherwise produces an error Optional with `Error` set but no `ErrorCode`.
3. CodeErr
   - Flow: `(code, err) -> errorHandler? -> decision logic`.
   - Cases:
     - `code == 0 && err == nil` → empty (error already processed / ignored).
     - `code == PANIC_CODE` → `panic(err)`.
     - Else type switch:
       - `string` → `Error` from string (no `ErrorCode`).
       - `error` → `Error` set (no `ErrorCode`).
       - other → `unknownErrorHandler` or panic.
4. Cast
   - Forwards error 1:1 (including `ErrorCode`).
   - On success: attempts type assertion. Failure → PANIC_CODE (panic via `CodeErr`).
5. GoOpt
   - Converts `(value, error)`; on error the (possibly partially populated) `value` remains (`Value` + `Error`).
6. None
   - Neutral state (`Value` = zero value, no error). Used as success for `Optional[Void]`.
7. IsSome
   - Based solely on zero value; zero value ≠ error. Use `IsError()` for error checks.
8. PANIC_CODE
   - Reserved for hard escalation / assertions. Not for regular semantic error codes.
9. ErrorHandler
   - Enables mapping, normalization, escalation (`PANIC_CODE`), or consumption (`0,nil`).
10. UnknownErrorHandler
    - Converts non-standard error objects; if absent, unknown type causes panic.
11. ErrorCode field
    - Set only if provided explicitly by the handler or unknown handler; default `string` / `error` paths currently do not set it.

Recommendation: If you need to evaluate error codes, extend `CodeErr` for `string` / `error` inputs or generate errors exclusively via code paths.

## Global Error Hooks

```go
type ErrorHandler func(code uint32, err interface{}) (uint32, error)
type UnknownErrorHandler func(code uint32, err interface{}) (uint32, error)

func SetErrorHandler(h ErrorHandler)
func SetUnknownErrorHandler(h UnknownErrorHandler)
```

Use cases:

- Mapping / normalizing error codes (e.g. grouping, masking)
- Automatic logging / metrics
- Escalation of specific errors via `PANIC_CODE`
- Converting exotic error types into `error`

Example:

```go
SetErrorHandler(func(code uint32, err interface{}) (uint32, error) {
    if code == WARN_NON_CRITICAL { return 0, nil }       // swallow
    if code == FATAL_DB_CORRUPTION { return PANIC_CODE, fmt.Errorf("fatal: %v", err) }
    return code, fmt.Errorf("%v", err)
})
```

## Typical Usage

### 1. Simple success / error path

```go
func LoadConfig(path string) Optional[Config] {
    raw, err := os.ReadFile(path)
    if err != nil { return CodeErr[Config](ERROR_CONFIG_READ, err) }
    cfg, err := parse(raw)
    if err != nil { return CodeErr[Config](ERROR_CONFIG_PARSE, err) }
    return Ok(cfg)
}
```

### 2. Early returns & type anchoring with `Opt[T]`

`Opt[T]` exists solely to make the generic type T explicit for early `return` paths without a value (type anchoring). Common patterns use a local variable `var opt Opt[T]` and its methods for concise, consistent returns.

Typical scenarios:

1. Returning an error before a value exists
2. Multiple early abort paths in a function
3. Void operations (`Optional[Void]`) with success or error

Example (simplified from real pattern):

```go
func FindCustomerByToken(client *dynamodb.DynamoDB, token string) Optional[*HostMonitoringCustomer] {
    var opt Opt[*HostMonitoringCustomer]

    expr, err := buildExpression(token)
    if err != nil {
        return opt.Err(err) // Optional[*HostMonitoringCustomer] with error
    }

    result, err := client.Scan(expr.ToParams())
    if err != nil {
        return opt.Err(err)
    }
    if len(result.Items) != 1 {
        return opt.Err(fmt.Errorf("expected exactly 1 customer, got %d", len(result.Items)))
    }

    customer := decodeCustomer(result.Items[0])
    return Ok(customer) // regular success path doesn't need opt
}
```

Void operation (similar to `UpsertCustomer` / `SaveError`):

```go
func UpsertCustomer(client *dynamodb.DynamoDB, c *Customer) Optional[Void] {
    var opt Opt[Void]
    item, err := marshal(c)
    if err != nil { return opt.Err(err) }
    if err := putItem(client, item); err != nil { return opt.Err(err) }
    return opt.None() // equivalent to None[Void]()
}
```

Why not directly `Err[T](...)` / `CodeErr[T](...)`?

- Readability: `opt.Err(err)` clearly signals an early return for the same T.
- Avoids retyping the generic type for complex types (`Opt[[]*Customer]`) on every return.
- Uniform pattern for Err / CodeErr / None.

When `Opt[T]` is NOT needed:

- Success returns (value available) → `return Ok(value)`.
- Only one error path and simple type → `return Err[T](err)` is fine.

Quick reference of `Opt[T]` methods:

- `opt.Err(err)` → `Err[T](err)`
- `opt.CodeErr(code, err)` → `CodeErr[T](code, err)`
- `opt.None()` → empty optional (`None[T]()`)

This keeps functions compact with generic type boilerplate centralized.

### 3. Verkettung mit Cast

```go
func OpenAndRead(path string) Optional[string] {
    f := OpenFile(path)         // Optional[*os.File]
    if f.IsError() { return Cast[string](f) }
    defer f.Value.Close()

    data := ReadAll(f.Value)    // Optional[[]byte]
    if data.IsError() { return Cast[string](data) }
    return Ok(string(data.Value))
}
```

### 4. Bridging to classic Go code

```go
if opt := LoadConfig("conf.yaml"); opt.IsError() {
    return nil, opt.Error // or more specific mapping
} else {
    cfg, _ := opt.ToGo()
    return use(cfg)
}
```

### 5. Handling zero values

```go
o := Ok(0)          // int
o.IsSome()          // false (zero value) → NOT an error
o.IsError()         // false → success
```

## Pattern for error propagation

```go
res := SomeOp()
if res.IsError() { return Cast[TargetType](res) }
v := res.Value
```

## Void returns

```go
func StopService(name string) Optional[Void] {
    m := OpenScm()
    if m.IsError() { return Cast[Void](m) }
    s := OpenService(m.Value, name)
    if s.IsError() { return Cast[Void](s) }
    if err := s.Value.Stop(); err != nil { return CodeErr[Void](ERROR_SERVICE_STOP, err) }
    return None[Void]()
}
```

## Anti-Patterns

| Anti-Pattern                                              | Why it's bad                | Better                                                  |
| --------------------------------------------------------- | --------------------------- | ------------------------------------------------------- |
| `if opt.IsSome() { ... } else { ... }` for error checking | Zero value ≠ error          | `if opt.IsError() { ... }`                              |
| Directly accessing `Value` without checking               | Panics / wrong assumptions  | First check `IsError()` or consciously use `Unwrap()`   |
| Using `Cast` to "try" incompatible types                  | Leads to panic (PANIC_CODE) | Write explicit conversion                               |
| Returning `None()` to ignore an error                     | Obscures root cause         | Propagate error or intentionally map via `errorHandler` |

## Advanced Examples

### Custom mapping & logging

```go
SetErrorHandler(func(code uint32, err interface{}) (uint32, error) {
    log.Printf("code=%d err=%v", code, err)
    // downgrade certain codes
    if code == ERROR_TEMPORARY_NET { return 0, nil }
    return code, fmt.Errorf("%v", err)
})
```

### Combination with standard library

```go
func ReadFileOpt(p string) Optional[string] {
    return GoOpt(os.ReadFile(p)). // ([]byte,error) → Optional[[]byte]
        // not method-chained, so manual intermediate step:
        // example kept minimal:
}
```

Wrapper for:

```go
func ReadFileOpt(p string) Optional[string] {
    data, err := os.ReadFile(p)
    if err != nil { return Err[string](err) }
    return Ok(data)
}
```

## Decision Tree (Short)

1. Success + value? → `return Ok(v)`
2. Success without value? → `return None[Void]()` or `return None[T]()`
3. Error without code? → `return Err[T](err)`
4. Error with code / mapping needed? → `return CodeErr[T](code, err)`
5. Forward previous Optional? → `return Cast[Target](prev)`
6. From `(T,error)`? → use `GoOpt(value, err)`

## Advantages (current implementation)

1. Type safety & compile-time generics
2. Unified error codes + central mapping
3. Composable error propagation (`Cast`) without duplicate logging
4. Optional escalation via `PANIC_CODE`
5. Bridge to classic Go (`ToGo`, `GoOpt`)
6. Clear semantics for void functions (`Optional[Void]`)
