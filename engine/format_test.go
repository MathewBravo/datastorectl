package engine

import (
	"strings"
	"testing"

	"github.com/MathewBravo/datastorectl/provider"
)

func TestFormatPlan(t *testing.T) {
	t.Run("format_create", func(t *testing.T) {
		res := &provider.Resource{ID: rid("svc", "a"), Body: provider.NewOrderedMap()}
		res.Body.Set("name", provider.StringVal("hello"))
		res.Body.Set("count", provider.IntVal(3))

		plan := &Plan{
			Changes: []ResourceChange{
				{ID: rid("svc", "a"), Type: ChangeCreate, Desired: res},
			},
		}

		got := FormatPlan(plan)

		if !strings.Contains(got, "+ svc.a (create)") {
			t.Errorf("expected create header, got:\n%s", got)
		}
		if !strings.Contains(got, `    name: "hello"`) {
			t.Errorf("expected name attr, got:\n%s", got)
		}
		if !strings.Contains(got, "    count: 3") {
			t.Errorf("expected count attr, got:\n%s", got)
		}
	})
}
