// Command neextract cooks the bundled Natural Earth 110m extracts
// next to the geography.naturalearth source package.
//
// Usage:
//
//	go run ./cmd/neextract -in /path/to/geojson -out internal/plugins/sources/naturalearth
//
// Expected input filenames (Natural Earth 110m GeoJSON):
//
//	ne_110m_admin_0_boundary_lines_land.geojson
//	ne_110m_populated_places.geojson
//	ne_110m_rivers_lake_centerlines.geojson
//	ne_110m_geography_regions_elevation_points.geojson
//	ne_110m_admin_0_countries.geojson
//
// To bump detail later, run the same cook against 50m files and keep
// the output schema. Public domain: naturalearthdata.com.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ion-command/ion-command/collector/internal/plugins/sources/naturalearth"
)

func main() {
	input := flag.String("in", "", "directory of Natural Earth 110m GeoJSON files")
	output := flag.String("out", "internal/plugins/sources/naturalearth", "directory to write borders.json, places.json, rivers.json")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "neextract: -in is required")
		os.Exit(2)
	}
	if err := naturalearth.CookExtracts(*input, *output); err != nil {
		fmt.Fprintf(os.Stderr, "neextract: %v\n", err)
		os.Exit(1)
	}
}
