package swagger_rest

import (
	"encoding/json"
)

// `Optional` represents a type that can be `undefined`, `null`, or have a
// value. This can be used to represent JSON payloads.
type Optional[T any] optional[T]

// We use an array to encode the state of the value, where a `nil` array
// represents `undefined`, an empty array represents `null`, and an array with
// one value represents that value.
type optional[T any] []T

func Of[T any](value T) Optional[T] {
	return Optional[T]{value}
}

func Undefined[T any]() Optional[T] {
	return nil
}

func Null[T any]() Optional[T] {
	return Optional[T]{}
}

func (v Optional[T]) Get() (T, bool) {
	var zero T
	if v == nil {
		return zero, false
	}
	if len(v) == 0 {
		return zero, false
	}
	return v[0], true
}

func (v Optional[T]) IsUndefined() bool {
	return v == nil
}

func (v Optional[T]) IsNull() bool {
	return v != nil && len(v) == 0
}

func (v *Optional[T]) UnmarshalJSON(data []byte) error {
	var newV *T
	err := json.Unmarshal(data, &newV)
	if err != nil {
		return err
	}
	if newV == nil {
		*v = []T{}
	} else {
		*v = Of(*newV)
	}
	return nil
}
