package output

import (
	"encoding/json"

	"github.com/MathewBravo/datastorectl/engine"
	"github.com/MathewBravo/datastorectl/provider"
)

// --- JSON plan ---

type jsonPlan struct {
	Changes   []jsonChange    `json:"changes"`
	Unmanaged []jsonUnmanaged `json:"unmanaged,omitempty"`
	Guards    []jsonGuard     `json:"guards,omitempty"`
	Summary   string          `json:"summary"`
}

type jsonUnmanaged struct {
	ID jsonResourceID `json:"id"`
}

type jsonGuard struct {
	Resource jsonResourceID `json:"resource"`
	Reason   string         `json:"reason"`
}

type jsonChange struct {
	ID      jsonResourceID  `json:"id"`
	Action  string          `json:"action"`
	Desired map[string]any  `json:"desired,omitempty"`
	Diffs   []jsonValueDiff `json:"diffs,omitempty"`
}

type jsonResourceID struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type jsonValueDiff struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Old  any    `json:"old,omitempty"`
	New  any    `json:"new,omitempty"`
}

// FormatPlanJSON renders a plan as structured JSON.
func FormatPlanJSON(plan *engine.Plan) ([]byte, error) {
	jp := jsonPlan{
		Changes: make([]jsonChange, 0),
		Summary: plan.Summary(),
	}

	for _, c := range plan.Changes {
		if c.Type == engine.ChangeNoOp {
			continue
		}

		jc := jsonChange{
			ID:     toJSONID(c.ID),
			Action: c.Type.String(),
		}

		if c.Desired != nil && c.Desired.Body != nil {
			jc.Desired = bodyToMap(c.Desired.Body)
		}

		if len(c.Diff.Diffs) > 0 {
			jc.Diffs = make([]jsonValueDiff, len(c.Diff.Diffs))
			for i, d := range c.Diff.Diffs {
				jc.Diffs[i] = jsonValueDiff{
					Path: d.Path,
					Kind: d.Kind.String(),
				}
				if d.Kind == engine.DiffModified || d.Kind == engine.DiffRemoved {
					jc.Diffs[i].Old = valueToAny(d.Old)
				}
				if d.Kind == engine.DiffModified || d.Kind == engine.DiffAdded {
					jc.Diffs[i].New = valueToAny(d.New)
				}
			}
		}

		jp.Changes = append(jp.Changes, jc)
	}

	if len(plan.Unmanaged) > 0 {
		jp.Unmanaged = make([]jsonUnmanaged, len(plan.Unmanaged))
		for i, u := range plan.Unmanaged {
			jp.Unmanaged[i] = jsonUnmanaged{ID: toJSONID(u.ID)}
		}
	}

	if len(plan.Guards) > 0 {
		jp.Guards = make([]jsonGuard, len(plan.Guards))
		for i, g := range plan.Guards {
			jp.Guards[i] = jsonGuard{Resource: toJSONID(g.Resource), Reason: g.Reason}
		}
	}

	return json.MarshalIndent(jp, "", "  ")
}

// --- JSON apply result ---

type jsonApplyResult struct {
	Results []jsonResult `json:"results"`
	Summary string       `json:"summary"`
}

type jsonResult struct {
	ID     jsonResourceID `json:"id"`
	Status string         `json:"status"`
	Action string         `json:"action"`
	Error  string         `json:"error,omitempty"`
}

// FormatApplyResultJSON renders an apply result as structured JSON.
func FormatApplyResultJSON(result *engine.ApplyResult) ([]byte, error) {
	jr := jsonApplyResult{
		Results: make([]jsonResult, len(result.Results)),
		Summary: result.Summary(),
	}

	for i, r := range result.Results {
		jr.Results[i] = jsonResult{
			ID:     toJSONID(r.ID),
			Status: r.Status.String(),
			Action: r.ChangeType.String(),
		}
		if r.Error != nil {
			jr.Results[i].Error = r.Error.Error()
		}
	}

	return json.MarshalIndent(jr, "", "  ")
}

// --- helpers ---

func toJSONID(id provider.ResourceID) jsonResourceID {
	return jsonResourceID{Type: id.Type, Name: id.Name}
}

func bodyToMap(m *provider.OrderedMap) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, m.Len())
	for _, k := range m.Keys() {
		v, _ := m.Get(k)
		out[k] = valueToAny(v)
	}
	return out
}

func valueToAny(v provider.Value) any {
	switch v.Kind {
	case provider.KindString:
		return v.Str
	case provider.KindInt:
		return v.Int
	case provider.KindFloat:
		return v.Float
	case provider.KindBool:
		return v.Bool
	case provider.KindList:
		out := make([]any, len(v.List))
		for i, e := range v.List {
			out[i] = valueToAny(e)
		}
		return out
	case provider.KindMap:
		return bodyToMap(v.Map)
	default:
		return nil
	}
}
