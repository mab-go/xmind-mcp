package handler

import (
	"testing"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"
)

func TestDeepCloneTopicRemapSummaryAndBoundaryIDs(t *testing.T) {
	summaryTopicID := uuid.New().String()
	root := &xmind.Topic{
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
	clone, err := deepCloneTopic(root)
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == root.ID {
		t.Fatal("root id should change")
	}
	if len(clone.Children.Attached) != 1 || clone.Children.Attached[0].ID == "a-old" {
		t.Fatalf("attached child id should change: %+v", clone.Children.Attached[0])
	}
	if len(clone.Children.Summary) != 1 || clone.Children.Summary[0].ID == summaryTopicID {
		t.Fatalf("summary topic id should change: %+v", clone.Children.Summary[0])
	}
	if clone.Boundaries[0].ID == "bound-old" || clone.Boundaries[0].Range != "(0,0)" {
		t.Fatalf("boundary: want new id and preserved range, got %+v", clone.Boundaries[0])
	}
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
