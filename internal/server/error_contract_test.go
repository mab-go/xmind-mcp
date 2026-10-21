package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mab-go/xmind-mcp/internal/server/handler"
	"github.com/mab-go/xmind-mcp/internal/xmind"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// contractCase is how the error-contract tests call one tool. args (without path) must
// succeed on the contractFixture map; idKeys name the arguments that hold IDs, which the
// bogus-ID scenarios replace one at a time.
type contractCase struct {
	args    map[string]any
	idKeys  []string
	creates bool // the tool creates the file at path (xmind_create_map)
}

// contractCases lists every registered tool. Add each new tool here;
// TestToolErrorContractCoversAllTools fails for any registered tool missing from this table.
var contractCases = map[string]contractCase{
	"xmind_open_map":     {},
	"xmind_list_sheets":  {},
	"xmind_create_map":   {args: map[string]any{"root_title": "R"}, creates: true},
	"xmind_add_sheet":    {args: map[string]any{"title": "n1", "root_title": "R"}},
	"xmind_delete_sheet": {args: map[string]any{"sheet_id": "s2"}, idKeys: []string{"sheet_id"}},
	"xmind_get_subtree": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "p"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_get_topic_properties": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c1"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_search_topics": {
		args:   map[string]any{"sheet_id": "s1", "query": "c"},
		idKeys: []string{"sheet_id"},
	},
	"xmind_find_topic": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p", "title": "c1"},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_add_topic": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p", "title": "n1"},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_add_topics_bulk": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p", "topics": []any{map[string]any{"title": "n1"}}},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_duplicate_topic": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c1", "target_parent_id": "q"},
		idKeys: []string{"sheet_id", "topic_id", "target_parent_id"},
	},
	"xmind_rename_topic": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c1", "title": "n1"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_delete_topic": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c3"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_move_topic": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c3", "new_parent_id": "q"},
		idKeys: []string{"sheet_id", "topic_id", "new_parent_id"},
	},
	"xmind_reorder_children": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p", "ordered_ids": []any{"c3", "c2", "c1"}},
		idKeys: []string{"sheet_id", "parent_id", "ordered_ids"},
	},
	"xmind_set_topic_properties": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "c1", "notes": "n"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_set_topic_properties_bulk": {
		args:   map[string]any{"sheet_id": "s1", "topic_ids": []any{"c1", "c2"}, "notes": "n"},
		idKeys: []string{"sheet_id", "topic_ids"},
	},
	"xmind_add_floating_topic": {
		args:   map[string]any{"sheet_id": "s1", "title": "n1"},
		idKeys: []string{"sheet_id"},
	},
	"xmind_add_relationship": {
		args:   map[string]any{"sheet_id": "s1", "from_id": "c2", "to_id": "c3"},
		idKeys: []string{"sheet_id", "from_id", "to_id"},
	},
	"xmind_list_relationships": {
		args:   map[string]any{"sheet_id": "s1"},
		idKeys: []string{"sheet_id"},
	},
	"xmind_delete_relationship": {
		args:   map[string]any{"sheet_id": "s1", "relationship_id": "rel1"},
		idKeys: []string{"sheet_id", "relationship_id"},
	},
	"xmind_add_summary": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p", "from_index": 0, "to_index": 1},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_add_boundary": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "p"},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_flatten_to_outline": {
		args:   map[string]any{"sheet_id": "s1", "topic_id": "p"},
		idKeys: []string{"sheet_id", "topic_id"},
	},
	"xmind_import_from_outline": {
		args:   map[string]any{"sheet_id": "s1", "parent_id": "q", "outline": "x"},
		idKeys: []string{"sheet_id", "parent_id"},
	},
	"xmind_find_and_replace": {
		args:   map[string]any{"sheet_id": "s1", "find": "c1", "replace": "c1x"},
		idKeys: []string{"sheet_id"},
	},
}

// contractFixture returns the bytes of a small map with fixed IDs that every contractCases
// base call succeeds on. Sheet s1 has root r1 with children p (with c1, c2, c3) and q, and a
// relationship rel1 from c1 to c2; each topic's title is its ID. Sheet s2 exists so
// xmind_delete_sheet has a sheet it may delete.
func contractFixture(t *testing.T) []byte {
	t.Helper()
	topic := func(id string, children ...xmind.Topic) xmind.Topic {
		tp := xmind.Topic{ID: id, Title: id}
		if len(children) > 0 {
			tp.Children = &xmind.Children{Attached: children}
		}
		return tp
	}
	sheets := []xmind.Sheet{
		{
			ID: "s1", Title: "s1",
			RootTopic:     topic("r1", topic("p", topic("c1"), topic("c2"), topic("c3")), topic("q")),
			Relationships: []xmind.Relationship{{ID: "rel1", End1ID: "c1", End2ID: "c2"}},
		},
		{ID: "s2", Title: "s2", RootTopic: topic("r2")},
	}
	path := filepath.Join(t.TempDir(), "fixture.xmind")
	if err := xmind.CreateNewMap(path, sheets); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// contractEnv is one scenario's directory, the fixture bytes, and the tool being called.
type contractEnv struct {
	dir     string
	fixture []byte
	tc      contractCase
}

// call returns a copy of the tool's base arguments with path set.
func (e contractEnv) call(path any) map[string]any {
	args := maps.Clone(e.tc.args)
	if args == nil {
		args = map[string]any{}
	}
	args["path"] = path
	return args
}

// file writes data to name in the scenario's directory and returns its path.
func (e contractEnv) file(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(e.dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fixtureCall writes the fixture map to m.xmind and returns the base arguments for it, or for
// a new file beside it when the tool creates its file.
func (e contractEnv) fixtureCall(t *testing.T) map[string]any {
	t.Helper()
	path := e.file(t, "m.xmind", e.fixture)
	if e.tc.creates {
		path = filepath.Join(e.dir, "new.xmind")
	}
	return e.call(path)
}

// contractScenario is one wrong way to call a tool. Every tool must answer it with a tool
// error (never a JSON-RPC error) and leave the scenario's directory exactly as it was.
type contractScenario struct {
	name string
	// skip, when set, is why the scenario cannot run on this platform or as this user.
	skip string
	// writesOnly limits the scenario to tools that write (readOnlyHint false).
	writesOnly bool
	// want is a fragment the tool error must contain; "" accepts any message.
	want string
	// createWant is want for xmind_create_map; "" means create_map succeeds here, so the
	// scenario is skipped for it.
	createWant string
	// setup creates the scenario's files and returns the call's arguments.
	setup func(t *testing.T, e contractEnv) map[string]any
	// lock, when set, runs after the directory is snapshotted and before the call (a chmod),
	// and returns a function that undoes it before the directory is compared.
	lock func(t *testing.T, e contractEnv) (restore func())
}

// windowsSkip is the skip reason for scenarios that need Unix paths or permission bits.
func windowsSkip() string {
	if runtime.GOOS == "windows" {
		return "needs Unix paths and permission bits"
	}
	return ""
}

// unlessWindows returns reason as the skip reason everywhere except Windows.
func unlessWindows(reason string) string {
	if runtime.GOOS != "windows" {
		return reason
	}
	return ""
}

// permissionSkip is the skip reason for scenarios that need directory permissions enforced.
func permissionSkip() string {
	if s := windowsSkip(); s != "" {
		return s
	}
	if os.Geteuid() == 0 {
		return "root ignores directory permissions"
	}
	return ""
}

// onUnix returns fragment except on Windows, where the message differs and any is accepted.
func onUnix(fragment string) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return fragment
}

// chmodLock returns a scenario lock that sets the mode of rel (relative to the scenario's
// directory; "" is the directory itself) and restores restoreMode afterwards and at cleanup.
func chmodLock(rel string, mode, restoreMode fs.FileMode) func(*testing.T, contractEnv) func() {
	return func(t *testing.T, e contractEnv) func() {
		t.Helper()
		path := filepath.Join(e.dir, rel)
		restore := func() { _ = os.Chmod(path, restoreMode) }
		t.Cleanup(restore)
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return restore
	}
}

// zipWith returns a zip archive holding one entry.
func zipWith(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pathScenarios are the scenarios about the path argument: missing or mistyped, or naming
// something that is not a readable map.
func pathScenarios() []contractScenario {
	const missingPath = "missing required argument: path"
	return []contractScenario{
		{name: "path missing", want: missingPath, createWant: missingPath,
			setup: func(_ *testing.T, e contractEnv) map[string]any {
				args := e.call(nil)
				delete(args, "path")
				return args
			}},
		{name: "path not a string", want: "invalid argument path: expected a string",
			createWant: "invalid argument path: expected a string",
			setup:      func(_ *testing.T, e contractEnv) map[string]any { return e.call(5) }},
		{name: "path empty", want: missingPath, createWant: missingPath,
			setup: func(_ *testing.T, e contractEnv) map[string]any { return e.call("") }},
		{name: "no arguments", want: missingPath, createWant: missingPath,
			setup: func(*testing.T, contractEnv) map[string]any { return nil }},
		{name: "nonexistent file", want: "file not found",
			setup: func(_ *testing.T, e contractEnv) map[string]any { return e.call(filepath.Join(e.dir, "none.xmind")) }},
		{name: "missing folder", want: "file not found", createWant: "cannot create map",
			setup: func(_ *testing.T, e contractEnv) map[string]any {
				return e.call(filepath.Join(e.dir, "missing", "m.xmind"))
			}},
		{name: "directory", createWant: "file already exists",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				path := filepath.Join(e.dir, "d.xmind")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
				return e.call(path)
			}},
		{name: "not a zip", want: "read zip", createWant: "file already exists",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				return e.call(e.file(t, "m.xmind", []byte("not a zip")))
			}},
		{name: "zip without content.json", want: "missing content.json", createWant: "file already exists",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				return e.call(e.file(t, "m.xmind", zipWith(t, "metadata.json", "{}")))
			}},
		{name: "malformed content.json", createWant: "file already exists",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				return e.call(e.file(t, "m.xmind", zipWith(t, "content.json", "{")))
			}},
	}
}

// accessScenarios are the scenarios where the path cannot be reached: through a file, with an
// invalid character, without permission, through a symlink loop, or with too long a name.
func accessScenarios() []contractScenario {
	return []contractScenario{
		{name: "path through a file", want: onUnix("cannot access"), createWant: "cannot create map",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				return e.call(filepath.Join(e.file(t, "m.xmind", e.fixture), "child.xmind"))
			}},
		{name: "NUL in path", want: onUnix("cannot access"), createWant: onUnix("cannot create map"),
			setup: func(_ *testing.T, e contractEnv) map[string]any {
				return e.call(filepath.Join(e.dir, "m\x00.xmind"))
			}},
		{name: "permission denied", skip: permissionSkip(), want: "cannot access", createWant: "cannot create map",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				if err := os.Mkdir(filepath.Join(e.dir, "locked"), 0o755); err != nil {
					t.Fatal(err)
				}
				return e.call(e.file(t, filepath.Join("locked", "m.xmind"), e.fixture))
			},
			lock: chmodLock("locked", 0, 0o755)},
		{name: "symlink loop", skip: windowsSkip(), want: "cannot access", createWant: "cannot create map",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				// A path through the loop, not the loop itself: create_map's O_EXCL open of a
				// symlink reports EEXIST instead of ELOOP.
				if err := os.Symlink("loop", filepath.Join(e.dir, "loop")); err != nil {
					t.Fatal(err)
				}
				return e.call(filepath.Join(e.dir, "loop", "m.xmind"))
			}},
		{name: "name too long", skip: windowsSkip(), want: "cannot access", createWant: "cannot create map",
			setup: func(_ *testing.T, e contractEnv) map[string]any {
				return e.call(filepath.Join(e.dir, strings.Repeat("a", 250)+".xmind"))
			}},
		{name: "invalid name", skip: unlessWindows("* is a valid file name character outside Windows"),
			want: "cannot access", createWant: "cannot create map",
			setup: func(_ *testing.T, e contractEnv) map[string]any {
				return e.call(filepath.Join(e.dir, "m*.xmind"))
			}},
	}
}

// readOnlyScenarios are the scenarios where a writing tool cannot save: the map's folder or
// the map file itself is read-only.
func readOnlyScenarios() []contractScenario {
	return []contractScenario{
		{name: "read-only folder", skip: permissionSkip(), writesOnly: true,
			want: "cannot save map", createWant: "cannot create map",
			setup: func(t *testing.T, e contractEnv) map[string]any { return e.fixtureCall(t) },
			lock:  chmodLock("", 0o555, 0o755)},
		{name: "read-only file", skip: windowsSkip(), writesOnly: true,
			want: "the file is read-only", createWant: "file already exists",
			setup: func(t *testing.T, e contractEnv) map[string]any {
				return e.call(e.file(t, "m.xmind", e.fixture))
			},
			lock: chmodLock("m.xmind", 0o444, 0o644)},
	}
}

// idScenarios replace each of the tool's ID arguments, in turn, with an ID that does not exist
// and with a number.
func idScenarios(tc contractCase) []contractScenario {
	var out []contractScenario
	for _, key := range tc.idKeys {
		for _, bad := range []any{"bogus", 5} {
			out = append(out, contractScenario{
				name: fmt.Sprintf("%s=%v", key, bad),
				setup: func(t *testing.T, e contractEnv) map[string]any {
					args := e.fixtureCall(t)
					args[key] = replaceID(args[key], bad)
					return args
				},
			})
		}
	}
	return out
}

// replaceID returns bad in place of an ID argument. For a list of IDs it replaces only the last
// element, so the list keeps the length the tool checks first.
func replaceID(v, bad any) any {
	if ids, ok := v.([]any); ok {
		out := slices.Clone(ids)
		out[len(out)-1] = bad
		return out
	}
	return bad
}

// callContractTool sends tools/call for tool through s, so the tool middleware runs, and
// returns the decoded response. The request is built with json.Marshal, which escapes a NUL in
// a path as valid JSON (fmt's %q would not).
func callContractTool(t *testing.T, s *mcpserver.MCPServer, tool string, args map[string]any) map[string]any {
	t.Helper()
	params := map[string]any{"name": tool}
	if args != nil {
		params["arguments"] = args
	}
	msg, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params})
	if err != nil {
		t.Fatal(err)
	}
	return handleJSON(t, s, string(msg))
}

// resultText returns the first text content of a decoded tools/call result.
func resultText(result map[string]any) string {
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	return text
}

// assertToolError fails unless resp is a tool error (isError, not a JSON-RPC error) whose
// text contains want.
func assertToolError(t *testing.T, resp map[string]any, want string) {
	t.Helper()
	if rpcErr, ok := resp["error"]; ok {
		t.Fatalf("JSON-RPC error %v, want a tool error", rpcErr)
	}
	result, _ := resp["result"].(map[string]any)
	text := resultText(result)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("succeeded with %q, want a tool error", text)
	}
	if !strings.Contains(text, want) {
		t.Errorf("tool error %q, want it to contain %q", text, want)
	}
}

// snapshotDir maps each path under dir to its content (files), "dir" (directories) or
// "-> target" (symlinks), so comparing two snapshots catches any entry created, removed or
// changed, including leftover temp files and placeholders.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			snap[rel] = "-> " + target
			return err
		case d.IsDir():
			snap[rel] = "dir"
		default:
			data, err := os.ReadFile(path)
			snap[rel] = string(data)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// assertSameTree fails for each entry created, removed or changed between two snapshots.
func assertSameTree(t *testing.T, before, after map[string]string) {
	t.Helper()
	for _, path := range slices.Sorted(maps.Keys(after)) {
		if old, ok := before[path]; !ok {
			t.Errorf("created %s", path)
		} else if old != after[path] {
			t.Errorf("changed %s", path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("removed %s", path)
		}
	}
}

// writesFile reports whether a tool may write, from its readOnlyHint annotation. An
// unregistered tool (nil; TestToolErrorContractCoversAllTools reports it) counts as writing.
func writesFile(st *mcpserver.ServerTool) bool {
	if st == nil {
		return true
	}
	hint := st.Tool.Annotations.ReadOnlyHint
	return hint == nil || !*hint
}

// runContractScenario calls tool as sc describes and checks the contract: a tool error with
// the expected text, and the scenario's directory unchanged.
func runContractScenario(t *testing.T, s *mcpserver.MCPServer, tool string, writes bool, e contractEnv, sc contractScenario) {
	t.Helper()
	want := sc.want
	if e.tc.creates {
		want = sc.createWant
	}
	switch {
	case sc.skip != "":
		t.Skip(sc.skip)
	case e.tc.creates && want == "":
		t.Skip("xmind_create_map succeeds in this scenario")
	case sc.writesOnly && !writes:
		t.Skip("the tool does not write")
	}
	args := sc.setup(t, e)
	before := snapshotDir(t, e.dir)
	restore := func() {}
	if sc.lock != nil {
		restore = sc.lock(t, e)
	}
	resp := callContractTool(t, s, tool, args)
	restore()
	assertToolError(t, resp, want)
	assertSameTree(t, before, snapshotDir(t, e.dir))
}

// TestToolErrorContractCoversAllTools checks contractCases lists exactly the registered tools,
// and that each case's ID keys are among its base arguments.
func TestToolErrorContractCoversAllTools(t *testing.T) {
	tools := newXMindServer(context.Background(), handler.NewXMindHandler()).ListTools()
	for name := range tools {
		if _, ok := contractCases[name]; !ok {
			t.Errorf("tool %q has no entry in contractCases", name)
		}
	}
	for name, tc := range contractCases {
		if _, ok := tools[name]; !ok {
			t.Errorf("contractCases lists %q, which is not registered", name)
		}
		for _, key := range tc.idKeys {
			if _, ok := tc.args[key]; !ok {
				t.Errorf("%s: ID key %q is not in its base args", name, key)
			}
		}
	}
}

// TestToolErrorContractBaseline checks every base call succeeds on the fixture, so a scenario's
// tool error comes from what the scenario changed.
func TestToolErrorContractBaseline(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())
	fixture := contractFixture(t)
	for _, name := range slices.Sorted(maps.Keys(contractCases)) {
		t.Run(name, func(t *testing.T) {
			e := contractEnv{dir: t.TempDir(), fixture: fixture, tc: contractCases[name]}
			resp := callContractTool(t, s, name, e.fixtureCall(t))
			result, _ := resp["result"].(map[string]any)
			if isErr, _ := result["isError"].(bool); resp["error"] != nil || isErr {
				t.Fatalf("base call failed: %v", resp)
			}
		})
	}
}

// TestToolErrorContract calls every tool in every wrong way contractScenarios describes. Each
// call must return a tool error, not a JSON-RPC error, and leave its directory unchanged.
func TestToolErrorContract(t *testing.T) {
	s := newXMindServer(context.Background(), handler.NewXMindHandler())
	fixture := contractFixture(t)
	tools := s.ListTools()
	scenarios := slices.Concat(pathScenarios(), accessScenarios(), readOnlyScenarios())
	for _, name := range slices.Sorted(maps.Keys(contractCases)) {
		tc := contractCases[name]
		writes := writesFile(tools[name])
		for _, sc := range slices.Concat(scenarios, idScenarios(tc)) {
			t.Run(name+"/"+sc.name, func(t *testing.T) {
				runContractScenario(t, s, name, writes, contractEnv{dir: t.TempDir(), fixture: fixture, tc: tc}, sc)
			})
		}
	}
}
