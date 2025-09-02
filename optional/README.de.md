# Optional[T] – Einheitliches Fehler- & Rückgabepattern

Dieses Dokument beschreibt die aktuelle Implementierung der generischen `Optional[T]`-Struktur in `go-common/optional.go` und deren idiomatische Verwendung im Projekt.

## Motivation

Statt mehrfach `(T, error)` zu propagieren oder mit `panic/recover` zu arbeiten, kapselt `Optional[T]` Wert ODER Fehler (plus optionalen Fehlercode) in einem Typ und erlaubt:

- Explizite, erzwungene Fehlerbehandlung
- Einheitliche Semantik für Fehlercodes
- Einfache Weitergabe / Konvertierung zwischen Optional-Typen
- Optionale Integration mit zentralen Error-Handlern

## Kern-Typ

```go
type Optional[T any] struct {
    Value     T        // Enthaltener Wert (Zero-Value wenn nicht gesetzt oder Fehler)
    Error     error    // Fehlerobjekt (nil falls kein Fehler)
    ErrorCode uint32   // Fehlercode (0 falls kein Fehler)
}
```

Zusätzliche Sentinel-/Hilfstypen:

```go
type Void struct{}
type Opt[T any] struct{} // Fabrik zum „Typraten“ bei frühen Returns
```

Konstante:

```go
const PANIC_CODE = math.MaxUint32 // Reserviert: führt zu panic, wenn über CodeErr / Cast ausgelöst
```

## Methoden von Optional[T]

| Methode               | Zweck                                                                                    |
| --------------------- | ---------------------------------------------------------------------------------------- |
| `IsError() bool`      | Wahr, wenn `Error != nil` oder ein `ErrorCode != 0` vorliegt                             |
| `HasErrorCode() bool` | Wahr, wenn `ErrorCode != 0`                                                              |
| `IsSome() bool`       | Wahr, wenn `Value` nicht der Zero-Value des Typs ist (Achtung bei legitimen Zero-Werten) |
| `Unwrap() T`          | Gibt den Wert zurück oder `panic` bei Fehler                                             |
| `String() string`     | Wert oder Fehlermeldung als Text                                                         |
| `ToGo() (T, error)`   | Brücke zurück zum klassischen Go-Pattern                                                 |

Wichtig: Zur Fehlerprüfung immer `IsError()` nutzen – nicht `IsSome()`. Ein legitimer Zero-Wert (z.B. `0`, `""`, `nil` Slice) macht `IsSome()` sonst „false“ trotz Erfolg.

## Konstruktorfunktionen

```go
func Ok[T any](value T) Optional[T]                // Erfolgsfall
func Err[T any](err interface{}) Optional[T]       // Fehler ohne Code
func CodeErr[T any](code uint32, err interface{}) Optional[T] // Fehler mit Code & Handler-Kaskade
func Cast[T any, U any](another Optional[U]) Optional[T]       // Weiterreichen / ggf. Typkonversion
func GoOpt[T any](value T, err error) Optional[T]  // Aus klassischem (T,error)
func None[T any]() Optional[T]                     // Leer: weder Wert noch Fehler

// Fabrik-Hilfstyp (keine Instanz nötig)
func (Opt[T]) Err(err interface{}) Optional[T]
func (Opt[T]) CodeErr(code uint32, err interface{}) Optional[T]
func (Opt[T]) None() Optional[T]
```

### Verhalten & Besonderheiten

Kompakte Übersicht über das Laufzeitverhalten der Hilfsfunktionen und Felder:

1. Ok
   - Setzt nur `Value`; `Error` = nil; `ErrorCode` = 0.
2. Err
   - Wrapper für `CodeErr(0, err)`.
   - Wird ein `errorHandler` verwendet, kann dieser den Fehler konsumieren (`code=0, err=nil`) → leeres Optional.
   - Andernfalls entsteht ein Fehler-Optional mit gesetztem `Error`, aber ohne `ErrorCode`.
3. CodeErr
   - Ablauf: `(code, err) -> errorHandler? -> Entscheidungslogik`.
   - Fälle:
     - `code == 0 && err == nil` → leer (Fehler bereits verarbeitet / ignoriert).
     - `code == PANIC_CODE` → `panic(err)`.
     - Sonst Typ-Switch:
       - `string` → `Error` aus String (kein `ErrorCode`).
       - `error` → `Error` gesetzt (kein `ErrorCode`).
       - anderer Typ → `unknownErrorHandler` oder `panic`.
4. Cast
   - Fehler wird 1:1 (inkl. `ErrorCode`) weitergegeben.
   - Bei Erfolg: versuchte Typassertion. Scheitern → PANIC_CODE (führt zu Panic über `CodeErr`).
5. GoOpt
   - Konvertiert `(value, error)`; bei Fehler bleibt der (evtl. teilgefüllte) `value` im Optional (`Value` + `Error`).
6. None
   - Neutraler Zustand (`Value` = Zero-Value, kein Fehler). Verwendung für Erfolgsfall bei `Optional[Void]`.
7. IsSome
   - Rein auf Zero-Value-Basis; Zero-Value ≠ Fehler. Für Fehlerprüfung ausschließlich `IsError()` nutzen.
8. PANIC_CODE
   - Reserviert für harte Eskalationen / Assertions. Nicht für reguläre semantische Fehlercodes verwenden.
9. ErrorHandler
   - Dient Mapping, Normalisierung, Eskalation (`PANIC_CODE`) oder Konsum (`0,nil`).
10. UnknownErrorHandler
    - Konvertiert nicht-standardisierte Fehlerobjekte; fehlt er, führt unbekannter Typ zu `panic`.
11. ErrorCode Feld
    - Ist nur gesetzt, wenn explizit vom Handler oder vom UnknownErrorHandler geliefert; Standardpfade (`string`/`error`) setzen es aktuell nicht.

Empfehlung: Wenn Fehlercodes ausgewertet werden sollen, `CodeErr` bei `string`/`error` erweitern oder Fehler ausschließlich über Codepfade erzeugen.

## Globale Error Hooks

```go
type ErrorHandler func(code uint32, err interface{}) (uint32, error)
type UnknownErrorHandler func(code uint32, err interface{}) (uint32, error)

func SetErrorHandler(h ErrorHandler)
func SetUnknownErrorHandler(h UnknownErrorHandler)
```

Einsatzmöglichkeiten:

- Mapping / Normalisierung von Fehlercodes (z.B. gruppieren, maskieren)
- Automatisches Logging / Metriken
- Eskalation bestimmter Fehler via `PANIC_CODE`
- Konvertierung exotischer Fehlertypen in `error`

Beispiel:

```go
SetErrorHandler(func(code uint32, err interface{}) (uint32, error) {
    if code == WARN_NON_CRITICAL { return 0, nil }       // „Schlucken“
    if code == FATAL_DB_CORRUPTION { return PANIC_CODE, fmt.Errorf("fatal: %v", err) }
    return code, fmt.Errorf("%v", err)
})
```

## Typische Verwendung

### 1. Einfacher Erfolgs-/Fehlerpfad

```go
func LoadConfig(path string) Optional[Config] {
    raw, err := os.ReadFile(path)
    if err != nil { return CodeErr[Config](ERROR_CONFIG_READ, err) }
    cfg, err := parse(raw)
    if err != nil { return CodeErr[Config](ERROR_CONFIG_PARSE, err) }
    return Ok(cfg)
}
```

### 2. Frühe Rückgaben & Typraten mit `Opt[T]`

`Opt[T]` dient ausschließlich dazu, den generischen Typ T bei frühen `return`-Pfaden ohne Wert explizit zu machen (Typraten). Praxis-Muster aus dem Code basieren auf einer lokalen Variable `var opt Opt[T]` und nutzen deren Methoden für konsistente, kurze Rückgaben.

Typische Anwendungsfälle:

1. Rückgabe eines Fehlers ohne zuvor einen Wert zu haben
2. Mehrere frühe Abbruchpfade in einer Funktion
3. Void-Operationen (`Optional[Void]`) mit Erfolg oder Fehler

Beispiel (vereinfacht nach realem Pattern):

```go
func FindCustomerByToken(client *dynamodb.DynamoDB, token string) Optional[*HostMonitoringCustomer] {
    var opt Opt[*HostMonitoringCustomer]

    expr, err := buildExpression(token)
    if err != nil {
        return opt.Err(err) // Optional[*HostMonitoringCustomer] mit Fehler
    }

    result, err := client.Scan(expr.ToParams())
    if err != nil {
        return opt.Err(err)
    }
    if len(result.Items) != 1 {
        return opt.Err(fmt.Errorf("expected exactly 1 customer, got %d", len(result.Items)))
    }

    customer := decodeCustomer(result.Items[0])
    return Ok(customer) // regulärer Erfolgsweg braucht kein opt
}
```

Void-Operation (analog zu `UpsertCustomer` / `SaveError`):

```go
func UpsertCustomer(client *dynamodb.DynamoDB, c *Customer) Optional[Void] {
    var opt Opt[Void]
    item, err := marshal(c)
    if err != nil { return opt.Err(err) }
    if err := putItem(client, item); err != nil { return opt.Err(err) }
    return opt.None() // entspricht None[Void]()
}
```

Warum nicht direkt `Err[T](...)` / `CodeErr[T](...)`?

- Lesbarkeit: `opt.Err(err)` signalisiert klar „früher Rückgabepfad für denselben T“.
- Kein erneutes Austippen des generischen Typs bei komplexen Typen (`Opt[[]*Customer]`) in jedem Return.
- Einheitliches Muster für Err / CodeErr / None.

Wann `Opt[T]` NICHT nötig ist:

- Erfolgsrückgaben (Wert liegt vor) → `return Ok(value)`.
- Wenn es nur genau einen Fehlerpfad gibt und der Typ einfach ist → `return Err[T](err)` ist ausreichend.

Kurzreferenz Methoden von `Opt[T]`:

- `opt.Err(err)` → `Err[T](err)`
- `opt.CodeErr(code, err)` → `CodeErr[T](code, err)`
- `opt.None()` → leeres Optional (`None[T]()`)

Damit bleibt die Funktion kompakt und generischer Typkram zentral an einer Stelle.

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

### 4. Bridging zu klassischem Go-Code

```go
if opt := LoadConfig("conf.yaml"); opt.IsError() {
    return nil, opt.Error // oder genaueres Mapping
} else {
    cfg, _ := opt.ToGo()
    return use(cfg)
}
```

### 5. Umgang mit Zero-Values

```go
o := Ok(0)          // int
o.IsSome()          // false (Zero-Value) → NICHT als Fehler interpretieren
o.IsError()         // false → Erfolg
```

## Muster für Fehlerweitergabe

```go
res := SomeOp()
if res.IsError() { return Cast[TargetType](res) }
v := res.Value
```

## Void-Rückgaben

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

| Anti-Pattern                                             | Warum schlecht                | Besser                                                     |
| -------------------------------------------------------- | ----------------------------- | ---------------------------------------------------------- |
| `if opt.IsSome() { ... } else { ... }` zur Fehlerprüfung | Zero-Value ≠ Fehler           | `if opt.IsError() { ... }`                                 |
| Direkter Zugriff auf `Value` ohne Prüfung                | Panics / falsche Annahmen     | Erst `IsError()` checken oder `Unwrap()` bewusst verwenden |
| `Cast` für inkompatible Typen „ausprobieren“             | Führt zu `panic` (PANIC_CODE) | Explizite Konvertierung schreiben                          |
| Fehler ignorieren, indem `None()` zurückgegeben wird     | Verschleiert Ursache          | Fehler weiterreichen oder bewusst mappen (`errorHandler`)  |

## Erweiterte Beispiele

### Custom Mapping & Logging

```go
SetErrorHandler(func(code uint32, err interface{}) (uint32, error) {
    log.Printf("code=%d err=%v", code, err)
    // bestimmte Codes herunterstufen
    if code == ERROR_TEMPORARY_NET { return 0, nil }
    return code, fmt.Errorf("%v", err)
})
```

### Kombination mit Standardbibliothek

```go
func ReadFileOpt(p string) Optional[string] {
    return GoOpt(os.ReadFile(p)). // ([]byte,error) → Optional[[]byte]
        // nicht method-chained, daher Zwischenschritt manuell:
        // Beispiel korrekt:
}
```

Ist ein Wrapper für:

```go
func ReadFileOpt(p string) Optional[string] {
    data, err := os.ReadFile(p)
    if err != nil { return Err[string](err) }
    return Ok(data)
}
```

## Entscheidungsbaum (Kurz)

1. Erfolg + Wert? → `return Ok(v)`
2. Erfolg ohne Wert? → `return None[Void]()` oder `return None[T]()`
3. Fehler ohne Code? → `return Err[T](err)`
4. Fehler mit Code / Mapping nötig? → `return CodeErr[T](code, err)`
5. Vorheriges Optional weiterreichen? → `return Cast[Ziel](alt)`
6. Aus `(T,error)`? → `GoOpt(value, err)` benutzen

## Vorteile (aktuelle Implementierung)

1. Typsicherheit & Compile-Time Generics
2. Vereinheitlichte Fehlercodes + zentrales Mapping
3. Komponierbare Fehlerpropagation (`Cast`) ohne doppeltes Logging
4. Optionale Eskalation via `PANIC_CODE`
5. Brückenfunktion zu klassischem Go (`ToGo`, `GoOpt`)
6. Klare Semantik für void-Funktionen (`Optional[Void]`)
