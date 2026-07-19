package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ha "github.com/Xevion/go-ha"
)

func TestToCamelCase(t *testing.T) {
	cases := map[string]string{
		"kitchen":          "Kitchen",
		"input_boolean":    "InputBoolean",
		"living_room_lamp": "LivingRoomLamp",
		"a":                "A",
		"":                 "",
		// A leading digit is not a legal identifier start, so it is prefixed.
		"2nd_floor": "_2ndFloor",
		// Repeated and trailing underscores leave empty parts, which are dropped.
		"a__b": "AB",
		"a_":   "A",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, toCamelCase(in))
		})
	}
}

func TestToFieldName(t *testing.T) {
	assert.Equal(t, "Kitchen", toFieldName("light.kitchen"))
	assert.Equal(t, "PorchMotion", toFieldName("binary_sensor.porch_motion"))
	// A malformed id has no field name; render rejects rather than emits it.
	assert.Equal(t, "", toFieldName("no_dot"))
	assert.Equal(t, "", toFieldName("too.many.dots"))
}

func TestIncludes(t *testing.T) {
	// An include list is exclusive and overrides exclude.
	assert.True(t, includes("light", []string{"light"}, nil))
	assert.False(t, includes("switch", []string{"light"}, nil))
	assert.False(t, includes("light", []string{"switch"}, []string{"light"}))

	// With no include list, everything not excluded passes.
	assert.True(t, includes("light", nil, nil))
	assert.False(t, includes("light", nil, []string{"light"}))
	assert.True(t, includes("switch", nil, []string{"light"}))
}

func entity(id, state string) ha.EntityState {
	return ha.EntityState{EntityID: id, State: state}
}

// parseGenerated fails the test unless the rendered output is a valid Go file,
// and returns it for structural assertions. render already runs go/format, which
// rejects invalid syntax, so a parse failure here means format let something
// through.
func parseGenerated(t *testing.T, src []byte) {
	t.Helper()
	_, err := parser.ParseFile(token.NewFileSet(), "entities.go", src, parser.AllErrors)
	require.NoError(t, err, "generated source did not parse:\n%s", src)
}

func TestRenderProducesTypedConstants(t *testing.T) {
	out, err := render([]ha.EntityState{
		entity("light.kitchen", "on"),
		entity("light.hall", "off"),
		entity("switch.fan", "on"),
	}, nil, nil)
	require.NoError(t, err)
	parseGenerated(t, out)

	s := string(out)
	// A domain's id type comes from services.DomainIDTypes.
	assert.Contains(t, s, "Kitchen services.LightID")
	assert.Contains(t, s, "Fan services.SwitchID")
	assert.Contains(t, s, `Kitchen: "light.kitchen"`)
	assert.Contains(t, s, `import "github.com/Xevion/go-ha/services"`)
}

func TestRenderUnknownDomainFallsBackToEntityID(t *testing.T) {
	out, err := render([]ha.EntityState{entity("weather.home", "sunny")}, nil, nil)
	require.NoError(t, err)
	parseGenerated(t, out)
	assert.Contains(t, string(out), "Home services.EntityID")
}

func TestRenderSkipsUnavailableAndMalformed(t *testing.T) {
	out, err := render([]ha.EntityState{
		entity("light.kitchen", "unavailable"),
		entity("light.hall", "on"),
		entity("no_domain", "on"),
	}, nil, nil)
	require.NoError(t, err)
	parseGenerated(t, out)

	s := string(out)
	assert.NotContains(t, s, "Kitchen", "an unavailable entity should be skipped")
	assert.NotContains(t, s, "NoDomain", "a malformed id should be skipped")
	assert.Contains(t, s, "Hall")
}

func TestRenderAppliesIncludeAndExclude(t *testing.T) {
	entities := []ha.EntityState{
		entity("light.kitchen", "on"),
		entity("switch.fan", "on"),
		entity("climate.hvac", "cool"),
	}

	included, err := render(entities, []string{"light"}, nil)
	require.NoError(t, err)
	assert.Contains(t, string(included), "light.kitchen")
	assert.NotContains(t, string(included), "switch.fan")
	assert.NotContains(t, string(included), "climate.hvac")

	excluded, err := render(entities, nil, []string{"switch"})
	require.NoError(t, err)
	assert.Contains(t, string(excluded), "light.kitchen")
	assert.NotContains(t, string(excluded), "switch.fan")
}

// The output is committed by users, so map iteration order must not leak into
// it. render sorts domains and fields; the same input must give the same bytes.
func TestRenderIsDeterministic(t *testing.T) {
	entities := []ha.EntityState{
		entity("switch.fan", "on"),
		entity("light.kitchen", "on"),
		entity("light.hall", "on"),
		entity("climate.hvac", "cool"),
	}

	first, err := render(entities, nil, nil)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		again, err := render(entities, nil, nil)
		require.NoError(t, err)
		require.Equal(t, first, again, "render output changed between runs")
	}

	// Fields within a domain are sorted, so Hall precedes Kitchen.
	s := string(first)
	assert.Less(t, strings.Index(s, "Hall"), strings.Index(s, "Kitchen"))
}

// Two ids that camel-case to the same field would emit a struct with a
// duplicate field, which does not compile. render has to catch this rather than
// hand the user broken code.
func TestRenderRejectsFieldCollision(t *testing.T) {
	_, err := render([]ha.EntityState{
		entity("light.a_b", "on"),
		entity("light.a__b", "on"),
	}, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "light.a_b")
	assert.Contains(t, err.Error(), "light.a__b")
}

func TestRenderEmptyInputIsValidEmptyPackage(t *testing.T) {
	out, err := render(nil, nil, nil)
	require.NoError(t, err)
	parseGenerated(t, out)

	s := string(out)
	assert.Contains(t, s, "package entities")
	// No domains means no import, or it would be unused and not compile.
	assert.NotContains(t, s, "import")
}
