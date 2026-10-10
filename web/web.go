package web

import (
	"bytes"
	"cmp"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	kenyageo "github.com/rmiyoyo/kenya-geo"
	"github.com/rmiyoyo/kenya-geo/api"
)

//go:embed templates static
var files embed.FS

type site struct {
	geo   *kenyageo.Data
	pages map[string]*template.Template
	maps  maps
	docs  *docs
}

type page struct {
	Title string
	Query string
	Data  any
}

var funcs = template.FuncMap{
	"num":             num,
	"decimal":         decimal,
	"round":           round,
	"coord":           coord,
	"km":              km,
	"title":           titleCase,
	"countyURL":       countyURL,
	"constituencyURL": constituencyURL,
	"wardURL":         wardURL,
	"postcodeURL":     postcodeURL,
	"atURL":           atURL,
	"list":            func(v ...any) []any { return v },
}

func New(geo *kenyageo.Data, docsURL string) http.Handler {
	s := &site{geo: geo, pages: map[string]*template.Template{}, maps: buildMaps(geo), docs: newDocs(docsURL)}
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		panic(err)
	}
	for _, name := range names {
		if name == "templates/layout.html" {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(name, "templates/"), ".html")
		s.pages[key] = template.Must(template.New("").Funcs(funcs).ParseFS(files, "templates/layout.html", name))
	}

	static, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /counties/{county}", s.county)
	mux.HandleFunc("GET /constituencies/{name}", s.constituency)
	mux.HandleFunc("GET /wards/{code}", s.ward)
	mux.HandleFunc("GET /postcodes/{code}", s.postcode)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("GET /at", s.at)
	mux.HandleFunc("GET /addresses", s.address)
	mux.HandleFunc("GET /docs", s.apiDocs)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.Handle("/api/", http.StripPrefix("/api", api.New(geo)))
	mux.HandleFunc("/", s.notFound)
	return mux
}

func (s *site) render(w http.ResponseWriter, r *http.Request, status int, name string, p page) {
	var buf bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&buf, "layout", p); err != nil {
		log.Printf("%s: rendering %s: %v", r.URL.Path, name, err)
		http.Error(w, "Something went wrong drawing this page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *site) notFound(w http.ResponseWriter, r *http.Request) {
	s.missing(w, r, "There is no page at "+r.URL.Path+".")
}

func (s *site) missing(w http.ResponseWriter, r *http.Request, msg string) {
	s.render(w, r, http.StatusNotFound, "missing", page{Title: "Not found", Data: msg})
}

type countyRow struct {
	kenyageo.County
	Voters kenyageo.VoterTotal
}

type homeData struct {
	Map            *mapView
	Counties       []countyRow
	Sort           string
	Voters         kenyageo.VoterTotal
	Constituencies int
	Wards          int
}

var sorts = map[string]func(a, b countyRow) int{
	"code":       func(a, b countyRow) int { return cmp.Compare(a.Code, b.Code) },
	"name":       func(a, b countyRow) int { return cmp.Compare(a.Name, b.Name) },
	"population": func(a, b countyRow) int { return cmp.Compare(b.Population, a.Population) },
	"area":       func(a, b countyRow) int { return cmp.Compare(b.AreaKm2, a.AreaKm2) },
	"density":    func(a, b countyRow) int { return cmp.Compare(b.Density(), a.Density()) },
	"voters": func(a, b countyRow) int {
		return cmp.Compare(b.Voters.RegisteredVoters2022, a.Voters.RegisteredVoters2022)
	},
}

func (s *site) home(w http.ResponseWriter, r *http.Request) {
	d := homeData{Map: s.maps.country, Sort: r.URL.Query().Get("sort"), Voters: s.geo.NationalVoters()}
	by, ok := sorts[d.Sort]
	if !ok {
		d.Sort, by = "code", sorts["code"]
	}
	for _, c := range s.geo.Counties() {
		v, _ := s.geo.CountyVoters(c.Code)
		d.Counties = append(d.Counties, countyRow{c, v})
		d.Constituencies += len(s.geo.Constituencies(c.Code))
		d.Wards += len(s.geo.WardsInCounty(c.Code))
	}
	slices.SortStableFunc(d.Counties, by)
	s.render(w, r, http.StatusOK, "home", page{Data: d})
}

type constituencyGroup struct {
	Name   string
	Voters kenyageo.VoterTotal
	Wards  []kenyageo.Ward
}

type countyData struct {
	County         kenyageo.County
	Map            *mapView
	Voters         kenyageo.VoterTotal
	Neighbours     []kenyageo.County
	Constituencies []constituencyGroup
	Wards          int
	PostOffices    []kenyageo.PostOffice
}

func (s *site) county(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("county")
	var c kenyageo.County
	var ok bool
	if code, err := strconv.Atoi(key); err == nil {
		c, ok = s.geo.CountyByCode(code)
	} else if c, ok = s.geo.CountyByName(key); ok {
		http.Redirect(w, r, countyURL(c.Code), http.StatusMovedPermanently)
		return
	}
	if !ok {
		s.missing(w, r, "There is no county called "+strconv.Quote(key)+".")
		return
	}
	d := countyData{
		County:      c,
		Map:         s.maps.counties[c.Code],
		Neighbours:  s.geo.NeighbouringCounties(c.Code),
		PostOffices: s.geo.PostOfficesInCounty(c.Code),
	}
	d.Voters, _ = s.geo.CountyVoters(c.Code)
	for _, name := range s.geo.Constituencies(c.Code) {
		g := constituencyGroup{Name: name, Wards: s.geo.WardsInConstituency(name)}
		g.Voters, _ = s.geo.ConstituencyVoters(name)
		d.Wards += len(g.Wards)
		d.Constituencies = append(d.Constituencies, g)
	}
	s.render(w, r, http.StatusOK, "county", page{Title: c.Name, Data: d})
}

type constituencyData struct {
	Name   string
	County kenyageo.County
	Map    *mapView
	Voters kenyageo.VoterTotal
	Wards  []kenyageo.Ward
	Others []string
}

func (s *site) constituency(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("name")
	wards := s.geo.WardsInConstituency(key)
	if len(wards) == 0 {
		s.missing(w, r, "There is no constituency called "+strconv.Quote(key)+".")
		return
	}
	d := constituencyData{Name: wards[0].Constituency, Wards: wards}
	d.County, _ = s.geo.CountyByCode(wards[0].CountyCode)
	d.Voters, _ = s.geo.ConstituencyVoters(d.Name)
	in := map[string]bool{}
	for _, w := range wards {
		in[w.Code] = true
	}
	d.Map = s.maps.counties[d.County.Code].highlight(func(k string) bool { return in[k] })
	for _, name := range s.geo.Constituencies(d.County.Code) {
		if name != d.Name {
			d.Others = append(d.Others, name)
		}
	}
	s.render(w, r, http.StatusOK, "constituency", page{Title: d.Name + " constituency", Data: d})
}

type wardData struct {
	Ward        kenyageo.Ward
	County      kenyageo.County
	Map         *mapView
	Siblings    []kenyageo.Ward
	PostOffices []kenyageo.NearbyPostOffice
}

func (s *site) ward(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	ward, ok := s.geo.WardByCode(code)
	if !ok {
		s.missing(w, r, "There is no ward with the code "+strconv.Quote(code)+".")
		return
	}
	d := wardData{Ward: ward}
	d.County, _ = s.geo.CountyByCode(ward.CountyCode)
	d.Map = s.maps.counties[ward.CountyCode].highlight(func(k string) bool { return k == ward.Code })
	for _, sib := range s.geo.WardsInConstituency(ward.Constituency) {
		if sib.Code != ward.Code {
			d.Siblings = append(d.Siblings, sib)
		}
	}
	if ward.Centroid != nil {
		d.PostOffices = s.geo.PostOfficesNear(ward.Centroid.Lat, ward.Centroid.Lng, 5)
	}
	s.render(w, r, http.StatusOK, "ward", page{Title: titleCase(ward.Name) + " ward", Data: d})
}

type located struct {
	kenyageo.PostOffice
	County kenyageo.County
	Ward   *kenyageo.Ward
}

type postcodeData struct {
	Code    string
	Offices []located
	Map     *mapView
}

func (s *site) postcode(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	offices := s.geo.PostOffices(code)
	if len(offices) == 0 {
		s.missing(w, r, "There is no post office with the postal code "+strconv.Quote(code)+".")
		return
	}
	d := postcodeData{Code: offices[0].Code}
	for _, p := range offices {
		l := located{PostOffice: p}
		l.County, _ = s.geo.CountyByCode(p.CountyCode)
		if p.Location != nil {
			if ward, ok := s.geo.WardAt(p.Location.Lat, p.Location.Lng); ok {
				l.Ward = &ward
			}
			if d.Map == nil {
				d.Map = s.maps.counties[p.CountyCode].mark(*p.Location)
			}
		}
		d.Offices = append(d.Offices, l)
	}
	s.render(w, r, http.StatusOK, "postcode", page{Title: d.Code + " " + offices[0].Name, Data: d})
}

type result struct {
	kenyageo.Match
	Href   string
	County string
}

func (s *site) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	var results []result
	for _, m := range s.geo.Search(q, 40) {
		res := result{Match: m}
		if c, ok := s.geo.CountyByCode(m.CountyCode); ok {
			res.County = c.Name
		}
		switch m.Kind {
		case kenyageo.KindCounty:
			res.Href = countyURL(m.CountyCode)
		case kenyageo.KindConstituency:
			res.Href = constituencyURL(m.Name)
		case kenyageo.KindWard:
			res.Href, res.Name = wardURL(m.Code), titleCase(m.Name)
		case kenyageo.KindPostOffice:
			res.Href = postcodeURL(m.Code)
		}
		results = append(results, res)
	}
	s.render(w, r, http.StatusOK, "search", page{Title: "Search for " + q, Query: q, Data: results})
}

type atData struct {
	Asked       bool
	Lat, Lng    string
	Problem     string
	Pick        *mapView
	Map         *mapView
	Ward        *kenyageo.Ward
	County      *kenyageo.County
	PostOffices []kenyageo.NearbyPostOffice
}

func (s *site) at(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := atData{Lat: q.Get("lat"), Lng: q.Get("lng"), Pick: s.maps.country.highlight(func(string) bool { return false })}
	if d.Pick != nil {
		d.Pick.Pick = true
	}
	status := http.StatusOK
	if d.Lat != "" || d.Lng != "" {
		d.Asked = true
		lat, err1 := strconv.ParseFloat(d.Lat, 64)
		lng, err2 := strconv.ParseFloat(d.Lng, 64)
		switch {
		case err1 != nil || err2 != nil:
			d.Problem, status = "Latitude and longitude must both be numbers, like -1.2884 and 36.8233.", http.StatusBadRequest
		default:
			s.locate(&d, lat, lng)
		}
	}
	s.render(w, r, status, "at", page{Title: "Find a place", Data: d})
}

func (s *site) locate(d *atData, lat, lng float64) {
	pt := kenyageo.LatLng{Lat: lat, Lng: lng}
	d.Pick = d.Pick.mark(pt)
	if ward, ok := s.geo.WardAt(lat, lng); ok {
		d.Ward = &ward
	}
	if c, ok := s.geo.CountyAt(lat, lng); ok {
		d.County = &c
	} else if d.Ward != nil {
		c, _ := s.geo.CountyByCode(d.Ward.CountyCode)
		d.County = &c
	}
	if d.County == nil {
		d.Problem = "That point isn't inside Kenya, as far as these boundaries go."
		return
	}
	d.Map = s.maps.counties[d.County.Code].mark(pt)
	if d.Ward != nil {
		d.Map = d.Map.highlight(func(k string) bool { return k == d.Ward.Code })
	}
	d.PostOffices = s.geo.PostOfficesNear(lat, lng, 5)
}

type addressData struct {
	Input    string
	Address  kenyageo.Address
	Problem  string
	Mismatch bool
	Examples []string
}

var addressExamples = []string{
	"P.O. Box 30197-00100 Nairobi",
	"Box 123, Nakuru 20100",
	"Private Bag 10400 Nanyuki",
	"P.O. Box 90-40100 Eldoret",
}

func (s *site) address(w http.ResponseWriter, r *http.Request) {
	d := addressData{Input: strings.TrimSpace(r.URL.Query().Get("q")), Examples: addressExamples}
	status := http.StatusOK
	if d.Input != "" {
		a, err := s.geo.ParseAddress(d.Input)
		d.Address = a
		switch {
		case errors.Is(err, kenyageo.ErrTownMismatch):
			d.Mismatch = true
			d.Problem = "The town doesn't match the postal code."
		case errors.Is(err, kenyageo.ErrNoBox):
			d.Problem = "There's no P.O. Box number or Private Bag in that address."
		case errors.Is(err, kenyageo.ErrUnknownPostOffice):
			d.Problem = "There's no post office with that postal code or town."
		case err != nil:
			d.Problem = err.Error()
		}
		if err != nil && !d.Mismatch {
			status = http.StatusUnprocessableEntity
		}
	}
	s.render(w, r, status, "address", page{Title: "Check an address", Data: d})
}

type docsData struct {
	Page    *docsPage
	Body    template.HTML
	Base    string
	Problem string
	Source  string
}

func (s *site) apiDocs(w http.ResponseWriter, r *http.Request) {
	d := docsData{Base: apiBase(r), Source: s.docs.url}
	doc, err := s.docs.get(r.Context())
	if err != nil {
		log.Printf("docs: %v", err)
	}
	if doc == nil {
		d.Problem = "The API documentation couldn't be loaded just now. Try again in a minute, or read it on GitHub."
		s.render(w, r, http.StatusBadGateway, "docs", page{Title: "API", Data: d})
		return
	}
	d.Page = doc
	d.Body = template.HTML(strings.ReplaceAll(string(doc.HTML), "localhost:8080/", template.HTMLEscapeString(d.Base)+"/"))
	s.render(w, r, http.StatusOK, "docs", page{Title: doc.Title, Data: d})
}

func apiBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api"
}
