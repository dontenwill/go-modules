package optional

import (
	"errors"
	"fmt"
	"math"
	"reflect"
)

const PANIC_CODE = math.MaxUint32

type Void struct{} // sentinel stating nothing is returned by a function. Optional[Void] infers that only error state can be returned.

//*********************************************************************************
//                               struct Optional
//*********************************************************************************

// Describes a return value that can either be a value of type T or an error.
type Optional[T any] struct {
	// Unsafely access the contained value. Equals zero memory if not set.
	Value T
	// Contains an error if the operation failed, nil otherwise.
	Error error
	// Contains the error code if the operation failed, 0 otherwise.
	ErrorCode uint32
}

// Returns if the Optional contains a value regardless of whether or not it contains an error.
func (o Optional[T]) IsSome() bool {
	return !reflect.ValueOf(o.Value).IsZero()
}

// Get the contained value, asserting that it exists.
func (o Optional[T]) Unwrap() T {
	if o.IsError() {
		panic(o.Error)
	}
	return o.Value
}

func (o Optional[T]) IsError() bool {
	return o.Error != nil || o.ErrorCode != 0
}

func (o Optional[T]) HasErrorCode() bool {
	return o.ErrorCode != 0
}

// String representation of the Optional, either the value or the error message.
// Used by logging and formatting macros.
func (o Optional[T]) String() string {
	if o.IsError() {
		return o.Error.Error()
	}
	return fmt.Sprintf("%v", o.Value)
}

// Convert to a tuple of (value, error) for use in traditional Go code.
func (o Optional[T]) ToGo() (T, error) {
	return o.Value, o.Error
}

//*********************************************************************************
//                              Optional Constructors
//*********************************************************************************

// Return a guaranteed value.
func Ok[T any](value T) Optional[T] {
	return Optional[T]{Value: value}
}

// Return an error without a code.
func Err[T any](err any) Optional[T] {
	return CodeErr[T](0, err)
}

// Return an error with a code.
func CodeErr[T any](code uint32, err any) Optional[T] {
	if errorHandler != nil {
		code, err = errorHandler(code, err)
	}
	if code == 0 && err == nil { // error has been handled?
		return Optional[T]{}
	}
	if code == PANIC_CODE {
		panic(err)
	}
	switch typed_err := err.(type) {
	case string:
		return Optional[T]{Error: errors.New(typed_err), ErrorCode: code}
	case error:
		return Optional[T]{Error: typed_err, ErrorCode: code}
	default:
		if unknownErrorHandler != nil {
			code, err := unknownErrorHandler(PANIC_CODE, typed_err)
			if code != PANIC_CODE {
				return Optional[T]{Error: err, ErrorCode: code}
			}
		}
		panic(fmt.Sprintf("<Optional[T]>.Err called with unknown error type %T", typed_err))
	}
}

// Pass the error or value from another Optional.
// If value is passed, it is converted if possible, otherwise an error is returned.
func Cast[T any, U any](another Optional[U]) Optional[T] {
	if another.IsError() {
		return Optional[T]{Error: another.Error, ErrorCode: another.ErrorCode}
	}
	if convertedValue, ok := any(another.Value).(T); ok {
		return Ok(convertedValue)
	} else {
		return CodeErr[T](PANIC_CODE, fmt.Errorf("Cast[%T](%T) failed. Types are not compatible", another.Value, convertedValue))
	}
}

// Convert a traditional Go (value, error) return to an Optional.
// Can wrap directly around a function call.
func GoOpt[T any](value T, err error) Optional[T] {
	if err != nil {
		opt := Err[T](err)
		opt.Value = value
		return opt
	}
	return Ok(value)
}

// Return an empty Optional, neither value nor error.
func None[T any]() Optional[T] {
	return Optional[T]{}
}

//*********************************************************************************
//                            Optional Factory (Opt)
//*********************************************************************************

// Do not instantiate.
// Use to infer type T without providing a value.
// Useful for functions that return Optional[T], but have multiple calls of return with no value.
// Ok() never needs to be constructed with Opt[T] because the value can always be inferred.
type Opt[T any] struct{}

func (Opt[T]) Err(err any) Optional[T]             { return Err[T](err) }
func (Opt[T]) CodeErr(code uint32, err any) Optional[T] { return CodeErr[T](code, err) }
func (Opt[T]) None() Optional[T]                            { return Optional[T]{} }

type ErrorHandler func(code uint32, err any) (uint32, error)
type UnknownErrorHandler func(code uint32, err any) (uint32, error)

var errorHandler ErrorHandler = nil
var unknownErrorHandler UnknownErrorHandler = nil

//*********************************************************************************
//                             Custom Error Handlers
//*********************************************************************************

// set an error handler that can modify and consume errors.
func SetErrorHandler(handler ErrorHandler) {
	errorHandler = handler
}

func SetUnknownErrorHandler(handler UnknownErrorHandler) {
	unknownErrorHandler = handler
}
