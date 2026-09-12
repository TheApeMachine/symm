package grid

import (
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ConditionToken preserves a quantity's identity and the directions of its level
relative to its causal baseline and its latest change. Zero, positive and
negative are exact order relations, not selected thresholds. Magnitude and
measurement quality remain in Space activity and observation authority.

Bit 52 distinguishes conditioned tokens from historical quantity IDs.
Four low bits hold the two ternary signs; the remaining 48 bits name a quantity,
keeping tokens exact in JSON/JavaScript.
*/
type TokenCommand struct {
	Condition *TokenCondition
	Remap     *TokenRemap
	Quantity  *TokenQuantity
}

/* TokenCondition asks for one quantity's fully conditioned token. */
type TokenCondition struct {
	Quantity uint64
	Level    float64
	Change   float64
}

/* TokenRemap replaces only the named quantity of one token. */
type TokenRemap struct {
	Token    uint64
	Quantity uint64
}

/* TokenQuantity recovers a token's quantity identity. */
type TokenQuantity struct {
	Token uint64
}

/*
TokenResult is one token command's answer: the token a condition built or
remapped, or the quantity a token named.
*/
type TokenResult struct {
	Token    uint64
	Quantity uint64
}

/*
Token is the condition-token Primitive: one owner for the token bit layout.
*/
type Token struct {
	err error
	out TokenResult
}

/* NewToken instantiates the condition-token Primitive. */
func NewToken() core.Primitive {
	return &Token{}
}

/*
Next executes each arriving token command and yields its result. An invalid
command is recorded in Error and ends the stream.
*/
func (op *Token) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*TokenCommand)(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Token) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one token command to its intent and returns its result.
*/
func (op *Token) execute(command *TokenCommand) (TokenResult, error) {
	intents := 0

	if command.Condition != nil {
		intents++
	}

	if command.Remap != nil {
		intents++
	}

	if command.Quantity != nil {
		intents++
	}

	if intents != 1 {
		return TokenResult{}, fmt.Errorf(
			"%w: grid: token command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Condition != nil {
		token, err := conditionToken(command.Condition.Quantity, command.Condition.Level, command.Condition.Change)

		if err != nil {
			return TokenResult{}, err
		}

		return TokenResult{Token: token}, nil
	}

	if command.Remap != nil {
		token, err := remapCondition(command.Remap.Token, command.Remap.Quantity)

		if err != nil {
			return TokenResult{}, err
		}

		return TokenResult{Token: token}, nil
	}

	return TokenResult{Quantity: conditionQuantity(command.Quantity.Token)}, nil
}

/* conditionToken builds one quantity's fully conditioned token. */
func conditionToken(quantity uint64, level, change float64) (uint64, error) {
	if quantity == 0 || quantity >= 1<<48 {
		return 0, fmt.Errorf(
			"%w: grid: condition quantity does not fit token encoding",
			core.ErrDomain,
		)
	}

	state := uint64(0)

	for index, value := range [2]float64{level, change} {
		if value > 0 {
			state |= 1 << (index * 2)
		}

		if value < 0 {
			state |= 2 << (index * 2)
		}
	}

	return 1<<52 | quantity<<4 | state, nil
}

/* remapCondition replaces only the named quantity when interning inputs. */
func remapCondition(token, quantity uint64) (uint64, error) {
	if token>>52 == 0 {
		return quantity, nil
	}

	if quantity == 0 || quantity >= 1<<48 {
		return 0, fmt.Errorf(
			"%w: grid: condition quantity does not fit token encoding",
			core.ErrDomain,
		)
	}

	return 1<<52 | quantity<<4 | token&15, nil
}

/*
conditionQuantity recovers the quantity identity without its directional bits.
Unconditioned quantity IDs pass through unchanged.
*/
func conditionQuantity(token uint64) uint64 {
	if token&(1<<52) == 0 {
		return token
	}

	return (token & ((1 << 52) - 1)) >> 4
}
