package site

import (
	"math"
	"slices"
	"strconv"

	kenyageo "github.com/rmiyoyo/kenya-geo"
)

type shape struct {
	Key  string
	Name string
	Href string
	D    string
	On   bool
}

type point struct {
	X, Y float64
}

type mapView struct {
	Label   string
	W, H    int
	Lng0    float64
	Lat0    float64
	Scale   float64
	Shapes  []shape
	Outline string
	Marker  *point
	Pick    bool
}

func newMap(label string, b [4]float64, maxW, maxH, pad float64) *mapView {
	dx, dy := b[2]-b[0], b[3]-b[1]
	s := min((maxW-2*pad)/dx, (maxH-2*pad)/dy)
	return &mapView{
		Label: label,
		W:     int(math.Ceil(dx*s + 2*pad)),
		H:     int(math.Ceil(dy*s + 2*pad)),
		Lng0:  b[0] - pad/s,
		Lat0:  b[3] + pad/s,
		Scale: s,
	}
}

func (m *mapView) xy(lng, lat float64) (float64, float64) {
	return (lng - m.Lng0) * m.Scale, (m.Lat0 - lat) * m.Scale
}

func (m *mapView) path(g kenyageo.Geometry, tolerance float64) string {
	var b []byte
	for _, polygon := range g.Coordinates {
		for _, ring := range polygon {
			pts := make([]point, 0, len(ring))
			for _, c := range ring {
				x, y := m.xy(c[0], c[1])
				if n := len(pts); n > 0 && math.Hypot(x-pts[n-1].X, y-pts[n-1].Y) < tolerance {
					continue
				}
				pts = append(pts, point{x, y})
			}
			if len(pts) < 3 {
				continue
			}
			for i, p := range pts {
				if i == 0 {
					b = append(b, 'M')
				} else {
					b = append(b, 'L')
				}
				b = strconv.AppendFloat(b, p.X, 'f', 1, 64)
				b = append(b, ' ')
				b = strconv.AppendFloat(b, p.Y, 'f', 1, 64)
			}
			b = append(b, 'Z')
		}
	}
	return string(b)
}

func (m *mapView) highlight(on func(key string) bool) *mapView {
	if m == nil {
		return nil
	}
	c := *m
	c.Shapes = slices.Clone(m.Shapes)
	for i := range c.Shapes {
		c.Shapes[i].On = on(c.Shapes[i].Key)
	}
	return &c
}

func (m *mapView) mark(loc kenyageo.LatLng) *mapView {
	if m == nil {
		return nil
	}
	c := *m
	x, y := m.xy(loc.Lng, loc.Lat)
	c.Marker = &point{x, y}
	return &c
}

func bounds(g kenyageo.Geometry) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	return extend(b, g)
}

func extend(b [4]float64, g kenyageo.Geometry) [4]float64 {
	for _, polygon := range g.Coordinates {
		if len(polygon) == 0 {
			continue
		}
		for _, c := range polygon[0] {
			b[0], b[1] = min(b[0], c[0]), min(b[1], c[1])
			b[2], b[3] = max(b[2], c[0]), max(b[3], c[1])
		}
	}
	return b
}

type maps struct {
	country  *mapView
	counties map[int]*mapView
}

func buildMaps(geo *kenyageo.Data) maps {
	out := maps{counties: map[int]*mapView{}}
	counties := geo.Counties()
	outlines := map[int]kenyageo.Geometry{}
	all := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, c := range counties {
		f, ok := geo.CountyBoundary(c.Code)
		if !ok {
			continue
		}
		outlines[c.Code] = f.Geometry
		all = extend(all, f.Geometry)
	}
	if len(outlines) == 0 {
		return out
	}

	out.country = newMap("Map of Kenya's 47 counties", all, 640, 720, 6)
	for _, c := range counties {
		if g, ok := outlines[c.Code]; ok {
			out.country.Shapes = append(out.country.Shapes, shape{
				Key:  strconv.Itoa(c.Code),
				Name: c.Name,
				Href: countyURL(c.Code),
				D:    out.country.path(g, 0.8),
			})
		}
	}

	for _, c := range counties {
		g, ok := outlines[c.Code]
		if !ok {
			continue
		}
		m := newMap("Map of the wards of "+c.Name, bounds(g), 640, 640, 6)
		m.Outline = m.path(g, 0.5)
		for _, w := range geo.WardsInCounty(c.Code) {
			f, ok := geo.WardBoundary(w.Code)
			if !ok {
				continue
			}
			m.Shapes = append(m.Shapes, shape{
				Key:  w.Code,
				Name: titleCase(w.Name),
				Href: wardURL(w.Code),
				D:    m.path(f.Geometry, 0.5),
			})
		}
		out.counties[c.Code] = m
	}
	return out
}
