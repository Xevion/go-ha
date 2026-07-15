package ha

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUndecided = errors.New("cannot tell")

func yes() Condition {
	return ConditionFunc(func(context.Context, EvalContext) (bool, error) { return true, nil })
}

func no() Condition {
	return ConditionFunc(func(context.Context, EvalContext) (bool, error) { return false, nil })
}

func broken() Condition {
	return ConditionFunc(func(context.Context, EvalContext) (bool, error) { return false, errUndecided })
}

func eval(t *testing.T, c Condition) (bool, error) {
	t.Helper()
	return c.Eval(context.Background(), EvalContext{Clock: testClock()})
}

func TestAll(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		want bool
	}{
		{"empty holds vacuously", All(), true},
		{"every condition holds", All(yes(), yes()), true},
		{"one fails", All(yes(), no()), false},
		{"all fail", All(no(), no()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eval(t, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAny(t *testing.T) {
	tests := []struct {
		name string
		cond Condition
		want bool
	}{
		{"empty holds for nothing", Any(), false},
		{"one holds", Any(no(), yes()), true},
		{"none hold", Any(no(), no()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := eval(t, tt.cond)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNot(t *testing.T) {
	got, err := eval(t, Not(yes()))
	require.NoError(t, err)
	assert.False(t, got)

	got, err = eval(t, Not(no()))
	require.NoError(t, err)
	assert.True(t, got)
}

// A condition that cannot be evaluated is undecided, not false. Reporting false
// would silently suppress an automation whose conditions were never checked.
func TestUndecidedPropagates(t *testing.T) {
	_, err := eval(t, broken())
	assert.ErrorIs(t, err, errUndecided)

	_, err = eval(t, Not(broken()))
	assert.ErrorIs(t, err, errUndecided, "the negation of undecided is still undecided")
}

// A definite answer settles the expression even when a sibling could not be
// evaluated, so an unreachable entity does not veto a decision already made.
func TestDefiniteAnswerBeatsAnUndecidedSibling(t *testing.T) {
	got, err := eval(t, All(broken(), no()))
	require.NoError(t, err, "false anywhere in All settles it")
	assert.False(t, got)

	got, err = eval(t, Any(broken(), yes()))
	require.NoError(t, err, "true anywhere in Any settles it")
	assert.True(t, got)
}

func TestUndecidedSurvivesWhenNothingElseSettles(t *testing.T) {
	_, err := eval(t, All(broken(), yes()))
	assert.ErrorIs(t, err, errUndecided, "All is undecided until something is false")

	_, err = eval(t, Any(broken(), no()))
	assert.ErrorIs(t, err, errUndecided, "Any is undecided until something is true")
}

func TestNestedComposition(t *testing.T) {
	got, err := eval(t, All(Any(no(), yes()), Not(no())))
	require.NoError(t, err)
	assert.True(t, got)
}
