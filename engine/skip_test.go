package engine

import "testing"

func TestShouldSkip(t *testing.T) {
	t.Run("no_failures", func(t *testing.T) {
		// A → B → C, no marks
		g := NewGraph()
		a, b, c := rid("x", "a"), rid("x", "b"), rid("x", "c")
		g.AddEdge(a, b)
		g.AddEdge(b, c)

		st := NewSkipTracker(g)
		if st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = false with no failures")
		}
	})

	t.Run("direct_dependency_failed", func(t *testing.T) {
		// A → B, mark B
		g := NewGraph()
		a, b := rid("x", "a"), rid("x", "b")
		g.AddEdge(a, b)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		if !st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = true when direct dep B failed")
		}
	})

	t.Run("transitive_dependency_failed", func(t *testing.T) {
		// A → B → C, mark C
		g := NewGraph()
		a, b, c := rid("x", "a"), rid("x", "b"), rid("x", "c")
		g.AddEdge(a, b)
		g.AddEdge(b, c)

		st := NewSkipTracker(g)
		st.MarkFailed(c)
		if !st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = true when transitive dep C failed")
		}
		if !st.ShouldSkip(b) {
			t.Fatal("expected ShouldSkip(B) = true when direct dep C failed")
		}
	})

	t.Run("independent_resource_unaffected", func(t *testing.T) {
		// A → B, C isolated, mark B
		g := NewGraph()
		a, b, c := rid("x", "a"), rid("x", "b"), rid("x", "c")
		g.AddEdge(a, b)
		g.AddNode(c)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		if st.ShouldSkip(c) {
			t.Fatal("expected ShouldSkip(C) = false for independent resource")
		}
	})

	t.Run("failed_resource_not_skipped", func(t *testing.T) {
		// A → B, mark B — B itself is failed, not skipped
		g := NewGraph()
		a, b := rid("x", "a"), rid("x", "b")
		g.AddEdge(a, b)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		if st.ShouldSkip(b) {
			t.Fatal("expected ShouldSkip(B) = false for the failed resource itself")
		}
	})

	t.Run("diamond_all_paths_fail", func(t *testing.T) {
		// D → B, D → C, B → A, C → A, mark A
		g := NewGraph()
		a, b, c, d := rid("x", "a"), rid("x", "b"), rid("x", "c"), rid("x", "d")
		g.AddEdge(d, b)
		g.AddEdge(d, c)
		g.AddEdge(b, a)
		g.AddEdge(c, a)

		st := NewSkipTracker(g)
		st.MarkFailed(a)
		if !st.ShouldSkip(d) {
			t.Fatal("expected ShouldSkip(D) = true in diamond with A failed")
		}
	})

	t.Run("diamond_partial_failure", func(t *testing.T) {
		// D → B, D → C, mark B
		g := NewGraph()
		b, c, d := rid("x", "b"), rid("x", "c"), rid("x", "d")
		g.AddEdge(d, b)
		g.AddEdge(d, c)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		if !st.ShouldSkip(d) {
			t.Fatal("expected ShouldSkip(D) = true when one diamond branch failed")
		}
	})

	t.Run("multiple_failures", func(t *testing.T) {
		// A → B, A → C, mark B and C
		g := NewGraph()
		a, b, c := rid("x", "a"), rid("x", "b"), rid("x", "c")
		g.AddEdge(a, b)
		g.AddEdge(a, c)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		st.MarkFailed(c)
		if !st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = true with multiple failed deps")
		}
	})

	t.Run("mark_failed_idempotent", func(t *testing.T) {
		// A → B, mark B twice — no panic, still skipped
		g := NewGraph()
		a, b := rid("x", "a"), rid("x", "b")
		g.AddEdge(a, b)

		st := NewSkipTracker(g)
		st.MarkFailed(b)
		st.MarkFailed(b)
		if !st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = true after idempotent MarkFailed")
		}
	})

	t.Run("no_dependencies", func(t *testing.T) {
		// A isolated, no marks
		g := NewGraph()
		a := rid("x", "a")
		g.AddNode(a)

		st := NewSkipTracker(g)
		if st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = false for isolated node")
		}
	})

	t.Run("long_chain", func(t *testing.T) {
		// A → B → C → D → E, mark E
		g := NewGraph()
		a, b, c, d, e := rid("x", "a"), rid("x", "b"), rid("x", "c"), rid("x", "d"), rid("x", "e")
		g.AddEdge(a, b)
		g.AddEdge(b, c)
		g.AddEdge(c, d)
		g.AddEdge(d, e)

		st := NewSkipTracker(g)
		st.MarkFailed(e)
		if !st.ShouldSkip(a) {
			t.Fatal("expected ShouldSkip(A) = true in long chain")
		}
		if !st.ShouldSkip(b) {
			t.Fatal("expected ShouldSkip(B) = true in long chain")
		}
		if !st.ShouldSkip(c) {
			t.Fatal("expected ShouldSkip(C) = true in long chain")
		}
		if !st.ShouldSkip(d) {
			t.Fatal("expected ShouldSkip(D) = true in long chain")
		}
	})
}
