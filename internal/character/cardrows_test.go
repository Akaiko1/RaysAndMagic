package character

import (
	"reflect"
	"testing"
)

func TestCardRowsKeepRolesAndDetailVisibility(t *testing.T) {
	section := CardSection{Title: "An unregistered heading"}
	section.AddDetail("Range: calculation\nsecond detail")
	section.Add("answer without a recognized prefix")
	hidden := CardSection{Title: "Hidden until expanded"}
	hidden.AddDetail("explanation")
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "compact", true: "full"}[full], func(t *testing.T) {
			rows := RenderCardRows([]CardSection{{Title: "Empty"}, section, hidden}, full)
			want := CardRows{
				{Text: section.Title, Kind: CardRowSection, Section: section.Title},
				{Text: "answer without a recognized prefix", Kind: CardRowResult, Section: section.Title},
			}
			if full {
				want = append(want,
					CardRow{Text: "Range: calculation", Kind: CardRowDetail, Section: section.Title},
					CardRow{Text: "second detail", Kind: CardRowDetail, Section: section.Title},
					CardRow{Kind: CardRowSpacer},
					CardRow{Text: hidden.Title, Kind: CardRowSection, Section: hidden.Title},
					CardRow{Text: "explanation", Kind: CardRowDetail, Section: hidden.Title},
				)
			}
			if !reflect.DeepEqual(rows, want) {
				t.Fatalf("roles/order/visibility = %+v, want %+v", rows, want)
			}
			if !reflect.DeepEqual(rows.Lines(), RenderCardLines([]CardSection{{Title: "Empty"}, section, hidden}, full)) {
				t.Fatal("plain-text projection differs from typed card")
			}
		})
	}
}
