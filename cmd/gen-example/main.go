// gen-example generates the example mind map used for the
// xmind-mcp README screenshot. It calls the xmind package directly — no MCP,
// no LLM — so the file can be regenerated at any time with:
//
//	go run ./cmd/gen-example [output-path]
//
// If no output path is given it defaults to ./example.xmind.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/mab-go/xmind-mcp/internal/xmind"
)

// structureClass matches what the XMind app writes for new maps (unbalanced radial).
// Do NOT use org.xmind.ui.map.clockwise — that forces a one-directional layout.
const structureClass = "org.xmind.ui.map.unbalanced"

func main() {
	outPath := "example.xmind"
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	sheets := []xmind.Sheet{buildSheet()}
	if err := xmind.CreateNewMap(outPath, sheets); err != nil {
		log.Fatalf("create map: %v", err)
	}
	fmt.Printf("wrote %s\n", outPath)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func id() string { return uuid.New().String() }

func topic(title string, children ...xmind.Topic) xmind.Topic {
	t := xmind.Topic{ID: id(), Title: title}
	if len(children) > 0 {
		t.Children = &xmind.Children{Attached: children}
	}
	return t
}

func leaves(titles ...string) []xmind.Topic {
	ts := make([]xmind.Topic, len(titles))
	for i, t := range titles {
		ts[i] = xmind.Topic{ID: id(), Title: t}
	}
	return ts
}

// ── map construction ─────────────────────────────────────────────────────────

func buildSheet() xmind.Sheet {

	// ── Sun ──────────────────────────────────────────────────────────────────
	sun := topic("☀️ Sun", leaves(
		"Type: G-type main-sequence star",
		"Age: ~4.6 billion years",
		"Diameter: 1,392,700 km",
		"Surface Temp: ~5,500°C",
	)...)
	sun.Markers = []xmind.Marker{{MarkerID: "star-yellow"}}

	// ── Inner Planets ─────────────────────────────────────────────────────────
	mercury := topic("Mercury", leaves(
		"Moons: None",
		"Diameter: 4,879 km",
		"Orbit: 88 days",
		"Closest to the Sun",
	)...)

	venus := topic("Venus", leaves(
		"Moons: None",
		"Diameter: 12,104 km",
		"Orbit: 225 days",
		"Hottest planet",
	)...)

	earthLeaves := leaves("Moon: Luna", "Diameter: 12,756 km", "Orbit: 365 days", "Only known life")
	earth := topic("Earth", earthLeaves...)
	earth.Labels = []string{"habitable zone"}
	earth.Markers = []xmind.Marker{{MarkerID: "flag-blue"}}

	// Keep a reference to the "Only known life" leaf for the relationship.
	onlyKnownLife := earthLeaves[3]

	mars := topic("Mars", leaves(
		"Moons: Phobos, Deimos",
		"Diameter: 6,792 km",
		"Orbit: 687 days",
		"Tallest volcano in solar system",
	)...)

	innerPlanets := topic("🪨 Inner Planets", mercury, venus, earth, mars)

	// Summary: "Rocky worlds" bracketing all four inner planets (indices 0–3).
	summaryTopicID := id()
	innerPlanets.Children.Summary = []xmind.Topic{{ID: summaryTopicID, Title: "Rocky worlds"}}
	innerPlanets.Summaries = []xmind.Summary{{
		ID:      id(),
		Range:   "(0,3)",
		TopicID: summaryTopicID,
	}}

	// ── Outer Planets ─────────────────────────────────────────────────────────
	jupiterMoons := xmind.Topic{ID: id(), Title: "Moons: Io, Europa, Ganymede, Callisto (+91)"}
	jupiter := topic("Jupiter",
		jupiterMoons,
		xmind.Topic{ID: id(), Title: "Diameter: 142,984 km"},
		xmind.Topic{ID: id(), Title: "Orbit: 12 years"},
		xmind.Topic{ID: id(), Title: "Great Red Spot"},
	)
	jupiter.Labels = []string{"gas giant"}
	jupiter.Markers = []xmind.Marker{{MarkerID: "priority-1"}}

	saturn := topic("Saturn", leaves(
		"Moons: Titan, Enceladus, Mimas (+143)",
		"Diameter: 120,536 km",
		"Orbit: 29 years",
		"Iconic ring system",
	)...)
	saturn.Labels = []string{"gas giant"}

	uranus := topic("Uranus", leaves(
		"Moons: Titania, Oberon, Miranda (+24)",
		"Diameter: 51,118 km",
		"Orbit: 84 years",
		"Rotates on its side",
	)...)
	uranus.Labels = []string{"ice giant"}

	neptune := topic("Neptune", leaves(
		"Moons: Triton (+15 others)",
		"Diameter: 49,528 km",
		"Orbit: 165 years",
		"Strongest winds in solar system",
	)...)
	neptune.Labels = []string{"ice giant"}

	outerPlanets := topic("🪐 Outer Planets", jupiter, saturn, uranus, neptune)

	// Boundary: "Gas & Ice Giants" around all outer planet children.
	outerPlanets.Boundaries = []xmind.Boundary{{
		ID:    id(),
		Range: "master",
		Title: "Gas & Ice Giants",
	}}

	// ── Dwarf Planets ─────────────────────────────────────────────────────────
	dwarfPlanets := topic("🌑 Dwarf Planets",
		topic("Pluto", leaves(
			"Moons: Charon, Nix, Hydra",
			"Orbit: 248 years",
			"Reclassified in 2006",
		)...),
		topic("Eris", leaves(
			"Moon: Dysnomia",
			"Orbit: 559 years",
			"Most massive dwarf planet",
		)...),
		topic("Ceres", leaves(
			"Moons: None",
			"Orbit: 4.6 years",
			"Only dwarf planet in asteroid belt",
		)...),
	)

	// ── Small Bodies ──────────────────────────────────────────────────────────
	smallBodies := topic("🪨 Small Bodies",
		topic("Asteroid Belt", leaves(
			"Between Mars and Jupiter",
			"Millions of rocky objects",
			"Largest: Ceres, Vesta, Pallas",
		)...),
		topic("Kuiper Belt", leaves(
			"Beyond Neptune",
			"Source of short-period comets",
			"Largest: Pluto, Eris, Makemake",
		)...),
		topic("Oort Cloud", leaves(
			"~2,000–100,000 AU from Sun",
			"Source of long-period comets",
			"Outer boundary of solar system",
		)...),
	)

	// ── Root ──────────────────────────────────────────────────────────────────
	root := xmind.Topic{
		ID:             id(),
		Class:          "topic",
		Title:          "The Solar System",
		StructureClass: structureClass,
		Children: &xmind.Children{
			Attached: []xmind.Topic{
				sun,
				innerPlanets,
				outerPlanets,
				dwarfPlanets,
				smallBodies,
			},
		},
	}

	// Relationship: "Only known life" (Earth) → Jupiter's moons, "Europa: potential life?"
	rel := xmind.Relationship{
		ID:     id(),
		End1ID: onlyKnownLife.ID,
		End2ID: jupiterMoons.ID,
		Title:  "Europa: potential life?",
	}

	return xmind.Sheet{
		ID:               id(),
		RevisionID:       id(),
		Class:            "sheet",
		Title:            "Solar System",
		TopicOverlapping: "overlap",
		RootTopic:        root,
		Relationships:    []xmind.Relationship{rel},
		Theme:            xmind.DefaultTheme,
		Extensions:       xmind.DefaultSheetExtensions(structureClass),
	}
}
