package ecd

import "errors"

// ErrValue is the refusal of a component value by the Entity Component Runtime codec: a value whose length is not its
// Size, or a byte that the codec does not write, such as a bool byte other than 0 and 1, a text length over its N, a
// byte after a text that is not zero, a NaN other than the canonical one or a negative zero. The Decode methods and the
// text constructors of generated code return errors that wrap it with the value's name and the reason. No engine
// answers it.
var ErrValue = errors.New("ecd: value refused")
