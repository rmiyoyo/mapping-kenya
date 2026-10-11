package site

import (
	_ "embed"
	"encoding/json"
	"strings"

	kenyageo "github.com/rmiyoyo/kenya-geo"
)

//go:embed lakes.json
var lakesJSON []byte

//go:embed ocean.json
var oceanJSON []byte

type lake struct {
	Name  string
	Label kenyageo.LatLng
	kenyageo.Geometry
}

type lakeShape struct {
	Name  string
	First string
	Rest  string
	D     string
	Label *point
}

var labels = map[string]kenyageo.LatLng{
	"Lake Victoria": {Lat: -0.62, Lng: 33.62},
	"Lake Turkana":  {Lat: 3.3, Lng: 36.75},
	"Indian Ocean":  {Lat: -3.3, Lng: 41.2},
}

func loadLakes(data []byte) []lake {
	var raw []struct {
		Name string `json:"name"`
		kenyageo.Geometry
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		panic(err)
	}
	out := make([]lake, len(raw))
	for i, r := range raw {
		out[i] = lake{Name: r.Name, Label: labels[r.Name], Geometry: r.Geometry}
	}
	return out
}

func overlaps(a, b [4]float64) bool {
	return a[0] < b[2] && b[0] < a[2] && a[1] < b[3] && b[1] < a[3]
}

func waterShapes(m *mapView, lakes []lake, view [4]float64, tolerance float64) []lakeShape {
	var out []lakeShape
	for _, l := range lakes {
		if !overlaps(bounds(l.Geometry), view) {
			continue
		}
		first, rest, _ := strings.Cut(l.Name, " ")
		s := lakeShape{Name: l.Name, First: first, Rest: rest, D: m.path(l.Geometry, tolerance)}
		if a := l.Label; a.Lng > view[0] && a.Lng < view[2] && a.Lat > view[1] && a.Lat < view[3] {
			x, y := m.xy(l.Label.Lng, l.Label.Lat)
			s.Label = &point{x, y}
		}
		out = append(out, s)
	}
	return out
}

func (m *mapView) addWater(lakes, sea []lake, view [4]float64, tolerance float64) {
	m.Lakes = waterShapes(m, lakes, view, tolerance)
	m.Sea = waterShapes(m, sea, view, tolerance)
}
