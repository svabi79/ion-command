package geoutil

import (
	"encoding/json"
	"testing"
)

func TestPointAndPolygonAndDecimate(t *testing.T) {
	body := []byte(`{"type":"FeatureCollection","features":[
		{"id":"p1","properties":{"name":"Pt"},"geometry":{"type":"Point","coordinates":[8.5,47.2]}},
		{"id":"a1","properties":{"name":"Box"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[2,0],[2,2],[0,2],[0,0]]]}}
	]}`)
	collection, err := ParseFeatureCollection(body)
	if err != nil || len(collection.Features) != 2 {
		t.Fatalf("%v %#v", err, collection)
	}
	lon, lat, ok := Point(collection.Features[0].Geometry)
	if !ok || lon != 8.5 || lat != 47.2 {
		t.Fatalf("point %v %v %v", lon, lat, ok)
	}
	polys := Polygons(collection.Features[1].Geometry)
	if len(polys) != 1 || len(polys[0][0]) != 5 {
		t.Fatalf("poly %#v", polys)
	}
	reduced := DecimateLine(polys[0][0], 50, 8)
	if len(reduced) < 4 {
		t.Fatalf("decimate %v", reduced)
	}
	if PropertyString(collection.Features[0].Properties, "name") != "Pt" {
		t.Fatal("property")
	}
}

func TestClosedLineBecomesPolygon(t *testing.T) {
	coords, _ := json.Marshal([][]float64{{0, 0}, {1, 0}, {1, 1}, {0, 0}})
	polys := Polygons(Geometry{Type: "LineString", Coordinates: coords})
	if len(polys) != 1 || len(polys[0][0]) < 4 {
		t.Fatalf("%#v", polys)
	}
}

func TestOpenLineStringIsNotAPolygon(t *testing.T) {
	coords, _ := json.Marshal([][]float64{{-90, 40}, {-88, 41}, {-86, 40}})
	if polys := Polygons(Geometry{Type: "LineString", Coordinates: coords}); len(polys) != 0 {
		t.Fatalf("open contour became polygon %#v", polys)
	}
}
