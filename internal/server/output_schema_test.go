package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/server/handler"
	"github.com/mab-go/xmind-mcp/internal/xmind"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// jsonOutputTools lists the tools whose handlers return jsonResult. Each must declare an
// outputSchema with handler.OutputSchema, and no other tool may declare one.
var jsonOutputTools = map[string]bool{
	"xmind_open_map":             true,
	"xmind_list_sheets":          true,
	"xmind_list_relationships":   true,
	"xmind_get_subtree":          true,
	"xmind_get_topic_properties": true,
	"xmind_search_topics":        true,
	"xmind_find_topic":           true,
	"xmind_add_topic":            true,
	"xmind_add_topics_bulk":      true,
	"xmind_duplicate_topic":      true,
	"xmind_move_topic":           true,
	"xmind_find_and_replace":     true,
}

func TestOutputSchemaDeclared(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())
	for name, st := range s.ListTools() {
		if got, want := len(st.Tool.RawOutputSchema) > 0, jsonOutputTools[name]; got != want {
			t.Errorf("%s: declares outputSchema = %v, want %v", name, got, want)
		}
		if schema := st.Tool.OutputSchema; schema.Type != "" || schema.Types != nil || len(schema.Properties) > 0 {
			t.Errorf("%s: OutputSchema is set; declare the schema with handler.OutputSchema", name)
		}
	}
	for name := range jsonOutputTools {
		if s.GetTool(name) == nil {
			t.Errorf("%s is in jsonOutputTools but not registered", name)
		}
	}
}

func TestStructuredOutputMatchesSchema(t *testing.T) {
	c := newOutputChecker(t)
	t.Run("map and sheet tools", func(t *testing.T) { checkSheetTools(t, c) })
	t.Run("topic shapes", func(t *testing.T) { checkTopicShapes(t, c) })
	t.Run("add_topic", func(t *testing.T) { checkAddTopic(t, c) })
	t.Run("add_topics_bulk", func(t *testing.T) { checkAddTopicsBulk(t, c) })
	t.Run("duplicate_topic", func(t *testing.T) { checkDuplicateTopic(t, c) })
	t.Run("move_topic", func(t *testing.T) { checkMoveTopic(t, c) })
	t.Run("find_and_replace", func(t *testing.T) { checkFindAndReplace(t, c) })
	t.Run("raw markerId", func(t *testing.T) { checkRawMarkerID(t, c) })
}

// checkRawMarkerID runs the tools that return markers on a topic whose markerId could not be
// decoded. The output must still match the schema, which wants a string markerId: it reads as
// "", and the marker's unknown key still shows.
func checkRawMarkerID(t *testing.T, c *outputChecker) {
	path := filepath.Join(t.TempDir(), "m.xmind")
	data := fixtureWithField(t, contractFixture(t), "c1", "markers", []any{map[string]any{"markerId": 5, "future": 1}})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"xmind_get_topic_properties", "xmind_get_subtree"} {
		out := c.call(t, tool, map[string]any{"path": path, "sheet_id": "s1", "topic_id": "c1"})
		if want := `"markers":[{"future":1,"markerId":""}]`; !strings.Contains(out, want) {
			t.Errorf("%s: output %s, want it to contain %s", tool, out, want)
		}
	}
}

func TestOutputSchemaRejectsUnknownKeys(t *testing.T) {
	c := newOutputChecker(t)
	for _, tc := range []struct {
		name, tool, payload string
		valid               bool
	}{
		{"marker keeps extra keys", "xmind_get_topic_properties", `{"id":"x","markers":[{"markerId":"m","future":1}]}`, true},
		{"unknown top-level key", "xmind_get_topic_properties", `{"id":"x","bogus":1}`, false},
		{"nested subtree", "xmind_get_subtree", `{"id":"a","children":[{"id":"b","children":[{"id":"c"}]}]}`, true},
		{"unknown key three levels down", "xmind_get_subtree", `{"id":"a","children":[{"id":"b","children":[{"id":"c","bogus":1}]}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := c.validate(tc.tool, []byte(tc.payload))
			if tc.valid && err != nil {
				t.Errorf("%s rejected %s: %v", tc.tool, tc.payload, err)
			}
			if !tc.valid && err == nil {
				t.Errorf("%s accepted %s", tc.tool, tc.payload)
			}
		})
	}
}

// outputChecker runs tools and checks each result against the tool's compiled outputSchema.
// It compiles the schemas itself: mcp-go's WithOutputSchemaValidation silently skips a schema
// that fails to compile.
type outputChecker struct {
	s       *mcpserver.MCPServer
	schemas map[string]*jsonschema.Schema
}

func newOutputChecker(t *testing.T) *outputChecker {
	t.Helper()
	s := newXMindServer(context.Background(), handler.NewXMindHandler())
	c := &outputChecker{s: s, schemas: map[string]*jsonschema.Schema{}}
	for name := range jsonOutputTools {
		st := s.GetTool(name)
		if st == nil {
			t.Fatalf("%s is not registered", name)
		}
		c.schemas[name] = compileOutputSchema(t, name, st.Tool.RawOutputSchema)
	}
	return c
}

func compileOutputSchema(t *testing.T, name string, raw json.RawMessage) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("%s: parse outputSchema: %v", name, err)
	}
	url := "mem:///" + name + ".json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(url, doc); err != nil {
		t.Fatalf("%s: add outputSchema: %v", name, err)
	}
	sch, err := compiler.Compile(url)
	if err != nil {
		t.Fatalf("%s: compile outputSchema: %v", name, err)
	}
	return sch
}

func (c *outputChecker) validate(tool string, data []byte) error {
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return c.schemas[tool].Validate(v)
}

// call runs a tool and checks that it succeeded, that its structuredContent matches the tool's
// outputSchema, and that its text content is the same JSON. It returns that JSON.
func (c *outputChecker) call(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: tool, Arguments: args}}
	res, err := c.s.GetTool(tool).Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s %v: %v", tool, args, err)
	}
	if res.IsError {
		t.Fatalf("%s %v: tool error: %v", tool, args, res.Content)
	}
	if res.StructuredContent == nil {
		t.Fatalf("%s %v: no structuredContent", tool, args)
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("%s %v: marshal structuredContent: %v", tool, args, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s %v: %d content items, want 1", tool, args, len(res.Content))
	}
	if text, ok := res.Content[0].(mcp.TextContent); !ok || text.Text != string(data) {
		t.Errorf("%s %v: text content differs from structuredContent\ntext:       %v\nstructured: %s", tool, args, res.Content[0], data)
	}
	if err := c.validate(tool, data); err != nil {
		t.Errorf("%s %v: structuredContent does not match outputSchema: %v\n%s", tool, args, err, data)
	}
	return string(data)
}

// copyKitchenSink copies the kitchen-sink fixture into t.TempDir() and returns the copy's path.
func copyKitchenSink(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "kitchen-sink.xmind"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "kitchen-sink.xmind")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write fixture copy: %v", err)
	}
	return path
}

func readSheets(t *testing.T, path string) []xmind.Sheet {
	t.Helper()
	sheets, err := xmind.ReadMap(path)
	if err != nil {
		t.Fatalf("read map: %v", err)
	}
	return sheets
}

// eachTopic calls visit for tp and every topic below it, with the name of the child list the
// topic is in ("" for tp itself).
func eachTopic(tp *xmind.Topic, list string, visit func(tp *xmind.Topic, list string)) {
	visit(tp, list)
	if tp.Children == nil {
		return
	}
	for _, l := range []struct {
		name   string
		topics []xmind.Topic
	}{
		{"attached", tp.Children.Attached},
		{"detached", tp.Children.Detached},
		{"summary", tp.Children.Summary},
	} {
		for i := range l.topics {
			eachTopic(&l.topics[i], l.name, visit)
		}
	}
}

// topicShape names the JSON keys tp has, its non-empty child lists, the child list it is in,
// and whether a relationship ends at it. Topics of the same shape give results of the same
// shape, so the test calls each tool once per shape rather than once per topic.
func topicShape(t *testing.T, tp *xmind.Topic, list string, related bool) string {
	t.Helper()
	data, err := json.Marshal(tp)
	if err != nil {
		t.Fatalf("marshal topic %s: %v", tp.ID, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal topic %s: %v", tp.ID, err)
	}
	keys := make([]string, 0, len(fields)+5)
	for k := range fields {
		keys = append(keys, k)
	}
	if ch := tp.Children; ch != nil {
		keys = append(keys, fmt.Sprintf("attached=%t detached=%t summary=%t",
			len(ch.Attached) > 0, len(ch.Detached) > 0, len(ch.Summary) > 0))
	}
	keys = append(keys, "in="+list, fmt.Sprintf("related=%t", related))
	slices.Sort(keys)
	return strings.Join(keys, ",")
}

func relationshipEnds(sh *xmind.Sheet) map[string]bool {
	ends := map[string]bool{}
	for _, rel := range sh.Relationships {
		ends[rel.End1ID] = true
		ends[rel.End2ID] = true
	}
	return ends
}

func checkSheetTools(t *testing.T, c *outputChecker) {
	path := copyKitchenSink(t)
	c.call(t, "xmind_open_map", map[string]any{"path": path})
	c.call(t, "xmind_list_sheets", map[string]any{"path": path})
	c.call(t, "xmind_search_topics", map[string]any{"path": path, "query": "e"})
	for _, sh := range readSheets(t, path) {
		c.call(t, "xmind_list_relationships", map[string]any{"path": path, "sheet_id": sh.ID})
		c.call(t, "xmind_search_topics", map[string]any{"path": path, "sheet_id": sh.ID, "query": "e"})
		// The sheet root has no parent, so ancestryPath and siblingTitles are null.
		c.call(t, "xmind_find_topic", map[string]any{"path": path, "sheet_id": sh.ID, "title": sh.RootTopic.Title})
		c.call(t, "xmind_get_subtree", map[string]any{
			"path": path, "sheet_id": sh.ID, "include_notes": true, "include_links": true,
		})
	}
}

// checkTopicShapes runs the per-topic read tools on one topic of every shape in the fixture.
func checkTopicShapes(t *testing.T, c *outputChecker) {
	path := copyKitchenSink(t)
	seen := map[string]bool{}
	for _, sh := range readSheets(t, path) {
		ends := relationshipEnds(&sh)
		eachTopic(&sh.RootTopic, "", func(tp *xmind.Topic, list string) {
			shape := topicShape(t, tp, list, ends[tp.ID])
			if seen[shape] {
				return
			}
			seen[shape] = true
			c.call(t, "xmind_get_topic_properties", map[string]any{"path": path, "sheet_id": sh.ID, "topic_id": tp.ID})
			c.call(t, "xmind_get_subtree", map[string]any{
				"path": path, "sheet_id": sh.ID, "topic_id": tp.ID,
				"depth": float64(1), "include_notes": true, "include_links": true,
			})
		})
	}
}

// writeFixture is a fresh kitchen-sink copy for one write case, with the first sheet whose root
// has at least two attached children, and the ids and title of those two children.
type writeFixture struct {
	path, sheetID string
	aID, aTitle   string
	bID           string
}

func newWriteFixture(t *testing.T) writeFixture {
	t.Helper()
	path := copyKitchenSink(t)
	for _, sh := range readSheets(t, path) {
		if sh.RootTopic.Children == nil || len(sh.RootTopic.Children.Attached) < 2 {
			continue
		}
		a, b := sh.RootTopic.Children.Attached[0], sh.RootTopic.Children.Attached[1]
		if a.Title == "" {
			continue
		}
		return writeFixture{path: path, sheetID: sh.ID, aID: a.ID, aTitle: a.Title, bID: b.ID}
	}
	t.Fatal("kitchen sink has no sheet whose root has two attached children")
	return writeFixture{}
}

func checkAddTopic(t *testing.T, c *outputChecker) {
	f := newWriteFixture(t)
	c.call(t, "xmind_add_topic", map[string]any{
		"path": f.path, "sheet_id": f.sheetID, "parent_id": f.aID, "title": "New Topic",
		"labels": []any{"l"}, "markers": []any{"priority-1"},
	})
}

func checkAddTopicsBulk(t *testing.T, c *outputChecker) {
	f := newWriteFixture(t)
	c.call(t, "xmind_add_topics_bulk", map[string]any{
		"path": f.path, "sheet_id": f.sheetID, "parent_id": f.aID,
		"topics": []any{
			map[string]any{"title": "Bulk 1", "children": []any{map[string]any{"title": "Bulk 1.1"}}},
			map[string]any{"title": "Bulk 2"},
		},
	})
}

func checkDuplicateTopic(t *testing.T, c *outputChecker) {
	f := newWriteFixture(t)
	c.call(t, "xmind_duplicate_topic", map[string]any{
		"path": f.path, "sheet_id": f.sheetID, "topic_id": f.aID, "target_parent_id": f.bID,
	})
}

func checkMoveTopic(t *testing.T, c *outputChecker) {
	f := newWriteFixture(t)
	args := map[string]any{"path": f.path, "sheet_id": f.sheetID, "topic_id": f.aID, "new_parent_id": f.bID}
	first := c.call(t, "xmind_move_topic", args)
	// The repeat finds the topic already last under b and takes the no-op path.
	if again := c.call(t, "xmind_move_topic", args); again != first {
		t.Errorf("repeated move = %s, want %s", again, first)
	}
}

func checkFindAndReplace(t *testing.T, c *outputChecker) {
	t.Run("no matches", func(t *testing.T) {
		f := newWriteFixture(t)
		got := c.call(t, "xmind_find_and_replace", map[string]any{"path": f.path, "find": "zz no such title zz", "replace": "x"})
		if want := `{"changedCount":0,"changes":null}`; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	})
	t.Run("one sheet", func(t *testing.T) {
		f := newWriteFixture(t)
		got := c.call(t, "xmind_find_and_replace", map[string]any{
			"path": f.path, "sheet_id": f.sheetID, "find": f.aTitle, "replace": f.aTitle + "!",
		})
		if !strings.Contains(got, `"newTitle"`) || strings.Contains(got, `"sheetId"`) {
			t.Errorf("want changes without sheetId, got %s", got)
		}
	})
	t.Run("all sheets", func(t *testing.T) {
		f := newWriteFixture(t)
		got := c.call(t, "xmind_find_and_replace", map[string]any{"path": f.path, "find": f.aTitle, "replace": f.aTitle + "!"})
		if !strings.Contains(got, `"sheetId"`) {
			t.Errorf("want changes with sheetId, got %s", got)
		}
	})
}
