package naturalearth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecimateAndDateline(t *testing.T) {
	long := [][]float64{{-10, 0}, {-9.1, 0}, {-8.2, 0}, {10, 0}}
	if got := decimateLine(long); len(got) < 2 || len(got) > len(long) {
		t.Fatalf("decimate %v", got)
	}
	crossed := [][]float64{{170, 10}, {179, 10}, {-175, 10}, {-160, 10}}
	parts := splitDateline(crossed)
	if len(parts) != 2 {
		t.Fatalf("expected dateline split, got %d %#v", len(parts), parts)
	}
}

func TestCookFromFixture(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ne_110m_admin_0_boundary_lines_land.geojson", `{
		"features":[
			{"properties":{"NE_ID":1,"NAME":"Demo","FEATURECLA":"International boundary (verify)"},
			 "geometry":{"type":"LineString","coordinates":[[0,0],[1,1],[2,1]]}},
			{"properties":{"NE_ID":2,"FEATURECLA":"Disputed (please verify)"},
			 "geometry":{"type":"LineString","coordinates":[[3,3],[4,4]]}}
		]}`)
	write("ne_110m_populated_places.geojson", `{
		"features":[
			{"properties":{"NAME":"Tokyo","FEATURECLA":"Admin-0 capital","POP_MAX":37732000,"SCALERANK":0,"NE_ID":9},
			 "geometry":{"type":"Point","coordinates":[139.75,35.68]}}
		]}`)
	write("ne_110m_geography_regions_elevation_points.geojson", `{
		"features":[
			{"properties":{"name":"Mount Everest","name_en":"Mount Everest","elevation":8848,"ne_id":1},
			 "geometry":{"type":"Point","coordinates":[86.92,27.99]}}
		]}`)
	write("ne_110m_admin_0_countries.geojson", `{
		"features":[
			{"properties":{"NAME":"Japan","ADM0_A3":"JPN","LABELRANK":2,"LABEL_X":138.0,"LABEL_Y":37.0}}
		]}`)
	write("ne_110m_rivers_lake_centerlines.geojson", `{
		"features":[
			{"properties":{"name":"Amazonas","name_en":"Amazon","scalerank":1,"featurecla":"River","ne_id":3},
			 "geometry":{"type":"LineString","coordinates":[[-70,-3],[-60,-3],[-50,-2]]}}
		]}`)
	out := t.TempDir()
	if err := CookExtracts(dir, out); err != nil {
		t.Fatal(err)
	}
	var borders []borderFeature
	if err := readJSON(filepath.Join(out, "borders.json"), &borders); err != nil || len(borders) != 1 {
		t.Fatalf("borders %v %#v", err, borders)
	}
	var places []placeFeature
	if err := readJSON(filepath.Join(out, "places.json"), &places); err != nil || len(places) != 3 {
		t.Fatalf("places %v %#v", err, places)
	}
	var rivers []riverFeature
	if err := readJSON(filepath.Join(out, "rivers.json"), &rivers); err != nil || len(rivers) != 1 || rivers[0].Name != "Amazon" {
		t.Fatalf("rivers %v %#v", err, rivers)
	}
}

func readJSON(path string, dest any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}
