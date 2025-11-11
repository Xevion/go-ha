package ha

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntervalBuildersDoNotShareATrigger(t *testing.T) {
	base := NewInterval().Call(nothing).Every("5m")

	early := base.StartingAt("06:00").Build()
	late := base.StartingAt("08:00").Build()

	require.NotNil(t, early.trigger)
	require.NotNil(t, late.trigger)

	assert.NotSame(t, early.trigger, late.trigger,
		"two intervals built from one chain must not share a trigger")
	assert.NotEqual(t, early.trigger.Hash(), late.trigger.Hash(),
		"a later StartingAt must not rewrite the epoch of an interval already built")
}
