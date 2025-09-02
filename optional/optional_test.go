package optional

import (
	"errors"
	"fmt"
	"testing"
)

// helper to ensure a function panics and to capture its value
func mustPanic(t *testing.T, f func()) any {
	 t.Helper()
	 defer func() {
		 if r := recover(); r == nil {
			 t.Fatalf("expected panic but none occurred")
		 }
	 }()
	 var got any
	 func() { // separate to capture recovered value cleanly
		 defer func() { got = recover(); panic(got) }()
		 f()
	 }()
	 return nil // unreachable
}

func TestOptional(t *testing.T) {
	 t.Run("Ok basic", func(t *testing.T) {
		 opt := Ok(42)
		 if opt.IsError() { t.Fatalf("unexpected error: %v", opt.Error) }
		 if !opt.IsSome() { t.Fatalf("expected IsSome true for non-zero value") }
		 if opt.Unwrap() != 42 { t.Fatalf("unwrap mismatch") }
	 })

	 t.Run("Ok zero value IsSome false but not error", func(t *testing.T) {
		 opt := Ok(0) // legitimate zero value
		 if opt.IsError() { t.Fatalf("unexpected error") }
		 if opt.IsSome() { t.Fatalf("IsSome should be false for zero value; indicates design caveat") }
	 })

	 t.Run("Err string", func(t *testing.T) {
		 opt := Err[int]("boom")
		 if !opt.IsError() { t.Fatalf("expected error") }
		 if opt.ErrorCode != 0 { t.Fatalf("unexpected code: %d", opt.ErrorCode) }
	 })

	 t.Run("CodeErr with code", func(t *testing.T) {
		 opt := CodeErr[int](1234, errors.New("x"))
		 if !opt.IsError() { t.Fatalf("expected error") }
		 if !opt.HasErrorCode() || opt.ErrorCode != 1234 { t.Fatalf("expected code 1234, got %d", opt.ErrorCode) }
	 })

	 t.Run("CodeErr PANIC_CODE panics", func(t *testing.T) {
		 mustPanic(t, func(){ CodeErr[int](PANIC_CODE, errors.New("panic")) })
	 })

	 t.Run("Cast success", func(t *testing.T) {
		 src := Ok(7)
		 dst := Cast[int](src)
		 if dst.IsError() { t.Fatalf("cast produced error: %v", dst.Error) }
		 if dst.Value != 7 { t.Fatalf("expected 7, got %v", dst.Value) }
	 })

	 t.Run("Cast incompatible panics", func(t *testing.T) {
		 src := Ok("hi")
		 mustPanic(t, func(){ Cast[int](src) })
	 })

	 t.Run("GoOpt with error keeps value", func(t *testing.T) {
		 val := 9
		 opt := GoOpt(val, errors.New("e"))
		 if !opt.IsError() { t.Fatalf("expected error") }
		 if opt.Value != val { t.Fatalf("value not preserved") }
	 })

	 t.Run("None", func(t *testing.T) {
		 opt := None[string]()
		 if opt.IsError() { t.Fatalf("none should not be error") }
		 if opt.IsSome() { t.Fatalf("none should not be some") }
	 })

	 t.Run("Unwrap panics on error", func(t *testing.T) {
		 opt := Err[int]("bad")
		 mustPanic(t, func(){ _ = opt.Unwrap() })
	 })

	 t.Run("ErrorHandler modifications", func(t *testing.T) {
		 prev := errorHandler
		 defer func(){ errorHandler = prev }()
		 SetErrorHandler(func(code uint32, err any) (uint32, error) {
			 return 77, fmt.Errorf("wrapped: %v", err)
		 })
		 opt := Err[int]("orig")
		 if opt.ErrorCode != 77 { t.Fatalf("expected modified code 77, got %d", opt.ErrorCode) }
		 if opt.Error == nil || opt.Error.Error() != "wrapped: orig" { t.Fatalf("unexpected error: %v", opt.Error) }
	 })

	 t.Run("ErrorHandler consumes error", func(t *testing.T) {
		 prev := errorHandler
		 defer func(){ errorHandler = prev }()
		 SetErrorHandler(func(code uint32, err any) (uint32, error) { return 0, nil })
		 opt := Err[int]("anything")
		 if opt.IsError() { t.Fatalf("expected consumed error -> no error") }
	 })

	 t.Run("UnknownErrorHandler handles unknown type", func(t *testing.T) {
		 prevUnknown := unknownErrorHandler
		 defer func(){ unknownErrorHandler = prevUnknown }()
		 SetUnknownErrorHandler(func(code uint32, err any) (uint32, error) {
			 // original code == PANIC_CODE, map to different code
			 if _, ok := err.(int); ok { return 555, fmt.Errorf("int: %v", err) }
			 return code, fmt.Errorf("unexpected")
		 })
		 opt := CodeErr[string](10, 123) // err is int (unknown type)
		 if !opt.IsError() { t.Fatalf("expected error") }
		 if opt.ErrorCode != 555 { t.Fatalf("expected remapped code 555, got %d", opt.ErrorCode) }
		 if opt.Error == nil || opt.Error.Error() != "int: 123" { t.Fatalf("unexpected mapped error: %v", opt.Error) }
	 })

	 t.Run("Opt generic factory methods", func(t *testing.T) {
		 // Use Opt[int] to create errors without repeating the type in the call site.
		 var oInt Opt[int]
		 errOpt := oInt.Err("fail")
		 if !errOpt.IsError() || errOpt.Error == nil { t.Fatalf("expected error via Opt.Err") }
		 if errOpt.ErrorCode != 0 { t.Fatalf("expected code 0, got %d", errOpt.ErrorCode) }

		 codeErrOpt := oInt.CodeErr(42, "boom")
		 if !codeErrOpt.IsError() || codeErrOpt.ErrorCode != 42 { t.Fatalf("expected code 42, got %d", codeErrOpt.ErrorCode) }

		 noneOpt := oInt.None()
		 if noneOpt.IsError() { t.Fatalf("noneOpt should not have error") }
		 if noneOpt.IsSome() { t.Fatalf("noneOpt should not be some (zero value)") }

		 // Demonstrate that value type is still int and zero-value retained
		 if noneOpt.Value != 0 { t.Fatalf("expected zero int value, got %v", noneOpt.Value) }

		 // Another type to ensure independence
		 var oStr Opt[string]
		 e2 := oStr.CodeErr(7, fmt.Errorf("x"))
		 if e2.ErrorCode != 7 || e2.Error == nil { t.Fatalf("expected string opt code 7") }
	 })
}
