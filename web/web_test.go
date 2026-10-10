package web

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	kenyageo "github.com/rmiyoyo/kenya-geo"
)

const sampleDocs = "# HTTP API\n\nIntro.\n\n## Wards\n\n### GET /wards/{code}\n\nOne ward.\n\n```sh\ncurl localhost:8080/wards/0040\n```\n\n| Parameter | Meaning |\n| --- | --- |\n| `lat` | latitude |\n\n<script>alert(1)</script>\n"

var docsFetches atomic.Int32

var docsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	docsFetches.Add(1)
	io.WriteString(w, sampleDocs)
}))

var handler = New(kenyageo.Default(), docsServer.URL)

func get(t *testing.T, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestPages(t *testing.T) {
	tests := []struct {
		path   string
		status int
		want   []string
	}{
		{"/", 200, []string{"Mombasa County", "22,096,344", `href="/counties/47"`, "<svg"}},
		{"/?sort=density", 200, []string{`aria-sort="descending"`}},
		{"/?sort=nonsense", 200, []string{"Mombasa County"}},
		{"/counties/32", 200, []string{"Nakuru County", "KE-31", "Rift Valley", "2,162,202", `href="/constituencies/Naivasha"`, `href="/counties/22"`}},
		{"/counties/99", 404, []string{"no county"}},
		{"/constituencies/Kibra", 200, []string{"Kibra constituency", "Nairobi County", `class="on"`}},
		{"/constituencies/Nowhere", 404, []string{"no constituency"}},
		{"/wards/0040", 200, []string{"Waa ward", "Matuga", "Kwale County", "20,681", `class="on"`}},
		{"/wards/9999", 404, []string{"no ward"}},
		{"/postcodes/00100", 200, []string{"Nairobi Gpo", "P.O. Box 123-00100"}},
		{"/postcodes/99999", 404, []string{"no post office"}},
		{"/search?q=kibera", 200, []string{"Kibra"}},
		{"/search?q=zzzzzzzzzz", 200, []string{"Nothing matches"}},
		{"/at", 200, []string{`class="map pick"`, "Latitude"}},
		{"/at?lat=-1.2884&lng=36.8233", 200, []string{"Nairobi Central", "Starehe", "Nairobi County", "City Square", `class="marker"`}},
		{"/at?lat=10&lng=10", 200, []string{"isn&#39;t inside Kenya"}},
		{"/at?lat=x&lng=1", 400, []string{"must both be numbers"}},
		{"/addresses", 200, []string{"Try one"}},
		{"/addresses?q=" + url.QueryEscape("P.O. Box 30197-00100 Nairobi"), 200, []string{"Nairobi Gpo", "P.O. Box 30197-00100 Nairobi"}},
		{"/addresses?q=" + url.QueryEscape("P.O. Box 90-40100 Eldoret"), 200, []string{"doesn&#39;t match", "40100 Kisumu"}},
		{"/addresses?q=hello", 422, []string{"no P.O. Box"}},
		{"/api/counties/47", 200, []string{`"name":"Nairobi County"`}},
		{"/static/site.css", 200, []string{"--paper"}},
		{"/static/site.js", 200, []string{"geolocation"}},
		{"/nope", 404, []string{"There is no page at /nope"}},
	}
	for _, tt := range tests {
		status, body := get(t, tt.path)
		if status != tt.status {
			t.Errorf("%s: status %d, want %d", tt.path, status, tt.status)
		}
		for _, w := range tt.want {
			if !strings.Contains(strings.ToLower(body), strings.ToLower(w)) {
				t.Errorf("%s: body lacks %q", tt.path, w)
			}
		}
	}
}

func TestRedirects(t *testing.T) {
	for path, want := range map[string]string{
		"/counties/nakuru": "/counties/32",
		"/search?q=":       "/",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if loc := rec.Header().Get("Location"); loc != want {
			t.Errorf("%s: redirected to %q, want %q", path, loc, want)
		}
	}
}

func TestEveryPlaceRenders(t *testing.T) {
	geo := kenyageo.Default()
	for _, c := range geo.Counties() {
		if status, _ := get(t, countyURL(c.Code)); status != 200 {
			t.Errorf("county %d: status %d", c.Code, status)
		}
		for _, name := range geo.Constituencies(c.Code) {
			if status, _ := get(t, constituencyURL(name)); status != 200 {
				t.Errorf("constituency %q: status %d", name, status)
			}
		}
		for _, w := range geo.WardsInCounty(c.Code) {
			if status, _ := get(t, wardURL(w.Code)); status != 200 {
				t.Errorf("ward %s: status %d", w.Code, status)
			}
		}
		for _, p := range geo.PostOfficesInCounty(c.Code) {
			if status, _ := get(t, postcodeURL(p.Code)); status != 200 {
				t.Errorf("post office %s: status %d", p.Code, status)
			}
		}
	}
}

func TestMapProjection(t *testing.T) {
	m := newMap("", [4]float64{34, -4, 42, 5}, 640, 720, 6)
	x, y := m.xy(34, 5)
	if math.Abs(x-6) > 1e-9 || math.Abs(y-6) > 1e-9 {
		t.Errorf("top left corner at %v,%v, want 6,6", x, y)
	}
	x, y = m.xy(42, -4)
	if x > float64(m.W) || y > float64(m.H) {
		t.Errorf("bottom right corner at %v,%v, outside %dx%d", x, y, m.W, m.H)
	}
	g := kenyageo.Geometry{Coordinates: [][][][2]float64{{{{34, 5}, {42, 5}, {42, -4}, {34, 5}}}}}
	if d := m.path(g, 0.5); !strings.HasPrefix(d, "M6.0 6.0L") || !strings.HasSuffix(d, "Z") {
		t.Errorf("path = %q", d)
	}
}

func TestFormat(t *testing.T) {
	for in, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 22096344: "22,096,344", -1500: "-1,500"} {
		if got := num(in); got != want {
			t.Errorf("num(%d) = %q, want %q", in, got, want)
		}
	}
	if got := decimal(8267.1); got != "8,267.1" {
		t.Errorf("decimal = %q", got)
	}
	for in, want := range map[string]string{"PORT REITZ": "Port Reitz", "MURANG'A": "Murang'a", "KAHAWA-WEST": "Kahawa-West"} {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDocs(t *testing.T) {
	before := docsFetches.Load()
	for range 3 {
		status, body := get(t, "/docs")
		if status != 200 {
			t.Fatalf("status %d", status)
		}
		for _, want := range []string{
			"<title>HTTP API · Kenya Geo</title>",
			`<a href="#get-wardscode">/wards/{code}</a>`,
			`<h3 id="get-wardscode">GET /wards/{code}</h3>`,
			"curl http://example.com/api/wards/0040",
			"<table>",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("/docs lacks %q", want)
			}
		}
		if strings.Contains(body, "alert(1)") {
			t.Error("/docs passed raw HTML from the Markdown through")
		}
		if strings.Count(body, "<h1>") != 1 {
			t.Error("/docs has more than one h1")
		}
	}
	if n := docsFetches.Load() - before; n > 1 {
		t.Errorf("fetched the docs %d times, want at most 1", n)
	}
}

func TestDocsUnavailable(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	defer down.Close()
	rec := httptest.NewRecorder()
	New(kenyageo.Default(), down.URL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "couldn") {
		t.Errorf("status %d\n%s", rec.Code, rec.Body)
	}
}
