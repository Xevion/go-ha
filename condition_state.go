package ha

import (
	"context"
	"fmt"
	"slices"
)

type stateIsCondition struct {
	entityID string
	states   []string
}

// StateIs holds while the entity's state is the given value.
func StateIs[T EntityRef](entityID T, state string) Condition {
	return stateIsCondition{entityID: string(entityID), states: []string{state}}
}

// StateIsOneOf holds while the entity's state is any of the given values.
func StateIsOneOf[T EntityRef](entityID T, states ...string) Condition {
	return stateIsCondition{entityID: string(entityID), states: states}
}

// StateIsNot holds while the entity's state is anything but the given value.
func StateIsNot[T EntityRef](entityID T, state string) Condition {
	return Not(StateIs(entityID, state))
}

func (c stateIsCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	entity, err := ec.State.Get(c.entityID)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", c.entityID, err)
	}
	return slices.Contains(c.states, entity.State), nil
}

func (c stateIsCondition) String() string {
	return fmt.Sprintf("%s is %v", c.entityID, c.states)
}
