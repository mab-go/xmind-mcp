package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"
)

func deepCloneFixtureTopic(summaryTopicID string) *xmind.Topic {
	return &xmind.Topic{
		ID: "root-old",
		Boundaries: []xmind.Boundary{
			{ID: "bound-old", Range: "(0,0)"},
		},
		Summaries: []xmind.Summary{
			{ID: "sum-old", Range: "(0,1)", TopicID: summaryTopicID},
		},
		Children: &xmind.Children{
			Attached: []xmind.Topic{{ID: "a-old", Title: "A"}},
			Summary:  []xmind.Topic{{ID: summaryTopicID, Title: "Sum"}},
		},
	}
}

func assertDeepCloneRootAndAttached(t *testing.T, root, clone *xmind.Topic, summaryTopicID string) {
	t.Helper()
	if clone.ID == root.ID {
		t.Fatal("root id should change")
	}
	if len(clone.Children.Attached) != 1 || clone.Children.Attached[0].ID == "a-old" {
		t.Fatalf("attached child id should change: %+v", clone.Children.Attached[0])
	}
	if len(clone.Children.Summary) != 1 || clone.Children.Summary[0].ID == summaryTopicID {
		t.Fatalf("summary topic id should change: %+v", clone.Children.Summary[0])
	}
}

func assertDeepCloneBoundaries(t *testing.T, clone *xmind.Topic) {
	t.Helper()
	if clone.Boundaries[0].ID == "bound-old" || clone.Boundaries[0].Range != "(0,0)" {
		t.Fatalf("boundary: want new id and preserved range, got %+v", clone.Boundaries[0])
	}
}

func assertDeepCloneSummaryDescriptors(t *testing.T, clone *xmind.Topic) {
	t.Helper()
	if clone.Summaries[0].ID == "sum-old" {
		t.Fatal("summary descriptor id should change")
	}
	if clone.Summaries[0].TopicID != clone.Children.Summary[0].ID {
		t.Fatalf("Summary.TopicID %q should equal summary topic id %q",
			clone.Summaries[0].TopicID, clone.Children.Summary[0].ID)
	}
	if clone.Summaries[0].Range != "(0,1)" {
		t.Fatalf("summary range should be preserved, got %q", clone.Summaries[0].Range)
	}
}

func TestDeepCloneTopicRemapSummaryAndBoundaryIDs(t *testing.T) {
	summaryTopicID := uuid.New().String()
	root := deepCloneFixtureTopic(summaryTopicID)
	clone, err := deepCloneTopic(root)
	if err != nil {
		t.Fatal(err)
	}
	assertDeepCloneRootAndAttached(t, root, &clone, summaryTopicID)
	assertDeepCloneBoundaries(t, &clone)
	assertDeepCloneSummaryDescriptors(t, &clone)
}

// nestedDetachedCloneFixture builds a topic that exercises clone paths the shallow
// deepCloneFixtureTopic does not: a detached subtree with a grandchild, and a nested
// (depth > 0) topic carrying both a boundary descriptor and a summary descriptor that
// references a summary topic in its own Children.Summary.
func nestedDetachedCloneFixture() *xmind.Topic {
	return &xmind.Topic{
		ID: "root-old",
		Children: &xmind.Children{
			Attached: []xmind.Topic{
				{
					ID: "a-old",
					Boundaries: []xmind.Boundary{
						{ID: "bound-nested-old", Range: "(0,0)"},
					},
					Summaries: []xmind.Summary{
						{ID: "sum-nested-old", Range: "(0,0)", TopicID: "ns-old"},
					},
					Children: &xmind.Children{
						Attached: []xmind.Topic{{ID: "a1-old", Title: "A1"}},
						Summary:  []xmind.Topic{{ID: "ns-old", Title: "NestedSum"}},
					},
				},
			},
			Detached: []xmind.Topic{
				{
					ID: "d-old",
					Children: &xmind.Children{
						Attached: []xmind.Topic{{ID: "d1-old", Title: "D1"}},
					},
				},
			},
		},
	}
}

func collectTopicIDs(root *xmind.Topic) []string {
	var ids []string
	walkTopics(root, 0, nil, func(t *xmind.Topic, _ int, _ *xmind.Topic) bool {
		ids = append(ids, t.ID)
		return true
	})
	return ids
}

func assertAllIDsRemappedAndUnique(t *testing.T, clone *xmind.Topic, oldIDs map[string]bool) {
	t.Helper()
	seen := make(map[string]bool)
	for _, id := range collectTopicIDs(clone) {
		if oldIDs[id] {
			t.Fatalf("old topic id %q survived the clone", id)
		}
		if seen[id] {
			t.Fatalf("duplicate topic id after clone: %q", id)
		}
		seen[id] = true
	}
}

func assertDetachedRemapped(t *testing.T, clone *xmind.Topic) {
	t.Helper()
	if clone.Children == nil || len(clone.Children.Detached) != 1 {
		t.Fatalf("expected one detached child, got %+v", clone.Children)
	}
	d := clone.Children.Detached[0]
	if d.ID == "d-old" {
		t.Fatal("detached topic id should change")
	}
	if d.Children == nil || len(d.Children.Attached) != 1 || d.Children.Attached[0].ID == "d1-old" {
		t.Fatalf("detached grandchild id should change: %+v", d.Children)
	}
}

func assertNestedBoundaryRemapped(t *testing.T, a *xmind.Topic) {
	t.Helper()
	if len(a.Boundaries) != 1 || a.Boundaries[0].ID == "bound-nested-old" || a.Boundaries[0].Range != "(0,0)" {
		t.Fatalf("nested boundary not remapped/preserved: %+v", a.Boundaries)
	}
}

func assertNestedSummaryRemapped(t *testing.T, a *xmind.Topic) {
	t.Helper()
	if len(a.Summaries) != 1 || a.Summaries[0].ID == "sum-nested-old" || a.Summaries[0].Range != "(0,0)" {
		t.Fatalf("nested summary descriptor not remapped/preserved: %+v", a.Summaries)
	}
	if a.Children == nil || len(a.Children.Summary) != 1 || a.Children.Summary[0].ID == "ns-old" {
		t.Fatalf("nested summary topic id should change: %+v", a.Children)
	}
	if a.Summaries[0].TopicID != a.Children.Summary[0].ID {
		t.Fatalf("nested Summary.TopicID %q should equal nested summary topic id %q",
			a.Summaries[0].TopicID, a.Children.Summary[0].ID)
	}
}

func assertNestedDescriptorsRemapped(t *testing.T, clone *xmind.Topic) {
	t.Helper()
	a := &clone.Children.Attached[0]
	assertNestedBoundaryRemapped(t, a)
	assertNestedSummaryRemapped(t, a)
}

// TestDeepCloneTopicDetachedAndNestedDescriptors pins issue #44 item 5: detached
// descendants are re-ID'd, boundary/summary descriptors on nested (depth > 0)
// topics are remapped, and a deeply nested Summary.TopicID rewrite stays consistent
// with its summary topic's new id.
func TestDeepCloneTopicDetachedAndNestedDescriptors(t *testing.T) {
	oldIDs := map[string]bool{
		"root-old": true, "a-old": true, "a1-old": true,
		"ns-old": true, "d-old": true, "d1-old": true,
	}
	clone, err := deepCloneTopic(nestedDetachedCloneFixture())
	if err != nil {
		t.Fatal(err)
	}
	assertAllIDsRemappedAndUnique(t, &clone, oldIDs)
	assertDetachedRemapped(t, &clone)
	assertNestedDescriptorsRemapped(t, &clone)
}

// danglingSummaryFixtureTopic builds a topic whose summary descriptor references a topicId that
// exists nowhere in its subtree (no matching Children.Summary entry).
func danglingSummaryFixtureTopic() *xmind.Topic {
	return &xmind.Topic{
		ID: "root-old",
		Summaries: []xmind.Summary{
			{ID: "sum-old", Range: "(0,0)", TopicID: "ghost-id"},
		},
		Children: &xmind.Children{
			Attached: []xmind.Topic{{ID: "a-old", Title: "A"}},
		},
	}
}

// TestDeepCloneTopicDanglingSummaryRef pins the dangling-reference failure to the typed
// cloneDanglingSummaryRefError so callers can classify it as a tool error (issue #46).
func TestDeepCloneTopicDanglingSummaryRef(t *testing.T) {
	_, err := deepCloneTopic(danglingSummaryFixtureTopic())
	if err == nil {
		t.Fatal("expected an error for a dangling summary reference, got nil")
	}
	var dangErr *cloneDanglingSummaryRefError
	if !errors.As(err, &dangErr) {
		t.Fatalf("expected *cloneDanglingSummaryRefError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "not found in cloned subtree") {
		t.Fatalf("unexpected message: %q", err.Error())
	}
}

func TestAncestryPathHelper(t *testing.T) {
	root := &xmind.Topic{ID: "r", Title: "Root"}
	if p := ancestryPath(root, root.ID); p != nil {
		t.Fatalf("root target: want nil, got %#v", p)
	}
	if p := ancestryPath(root, "no-such-id"); p != nil {
		t.Fatalf("missing id: want nil, got %#v", p)
	}

	child := &xmind.Topic{ID: "c", Title: "Child"}
	root.Children = &xmind.Children{Attached: []xmind.Topic{*child}}
	if got := ancestryPath(root, "c"); len(got) != 1 || got[0] != "Root" {
		t.Fatalf("direct child: got %#v", got)
	}

	deep := &xmind.Topic{ID: "d", Title: "Deep"}
	root.Children.Attached[0].Children = &xmind.Children{Attached: []xmind.Topic{*deep}}
	if got := ancestryPath(root, "d"); len(got) != 2 || got[0] != "Root" || got[1] != "Child" {
		t.Fatalf("deeper node: got %#v", got)
	}

	// Detached branch: second list after attached
	det := &xmind.Topic{ID: "det", Title: "Float"}
	root.Children.Detached = []xmind.Topic{*det}
	if got := ancestryPath(root, "det"); len(got) != 1 || got[0] != "Root" {
		t.Fatalf("detached child: got %#v", got)
	}
}
