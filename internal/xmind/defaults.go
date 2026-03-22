package xmind

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// DefaultTheme is the standard XMind theme applied to every new sheet.
// It is sourced from the kitchen-sink fixture (skeletonThemeId db4a5df4…,
// colorThemeId Rainbow-#000229-MULTI_LINE_COLORS) and matches what the XMind
// app writes when creating a new map with the default theme.
//
//go:embed default_theme.json
var DefaultTheme json.RawMessage

// DefaultSheetExtensions returns the standard org.xmind.ui.skeleton.structure.style
// extension block for a new sheet, referencing structureClass as the centralTopic layout.
func DefaultSheetExtensions(structureClass string) json.RawMessage {
	raw, err := marshalJSONNoHTMLEscape([]any{
		map[string]any{
			"provider": "org.xmind.ui.skeleton.structure.style",
			"content": map[string]string{
				"centralTopic": structureClass,
			},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("marshal default sheet extensions: %v", err))
	}
	return raw
}
