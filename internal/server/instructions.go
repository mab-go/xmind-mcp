package server

// serverInstructions is the usage guide sent in the initialize result; clients that
// support it add it to the model's context. It covers cross-tool workflow only: per-tool
// detail belongs in tool descriptions. Every connected session pays for it in context, so
// keep it within 150 words (TestInstructionsWordBudget) and name only registered tools
// (TestInstructionsNameRegisteredTools).
const serverInstructions = `xmind-mcp reads and edits local XMind (.xmind) files. Pass absolute paths; relative paths resolve against the server's working directory. There is no session or save step: a write tool's changes are on disk when it returns. The XMind app may overwrite on-disk changes when it saves, so before the first edit, tell the user once to close the map in XMind if it is open.

Typical workflow:
1. xmind_open_map (or xmind_create_map) to get sheet ids.
2. xmind_get_subtree with depth 1 to get the root topic id and its children, or xmind_find_topic / xmind_search_topics to locate topics by title.
3. Edit by topic id. Ids stay the same across edits, but titles can repeat.

Each write rewrites the whole file, so batch: xmind_add_topics_bulk adds a nested tree under one parent; xmind_set_topic_properties_bulk sets the same properties on many topics; xmind_import_from_outline adds outline text (without sheet_id, as a new sheet).`
