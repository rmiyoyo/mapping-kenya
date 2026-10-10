# mapping-kenya

A website for exploring Kenya's 47 counties, 290 constituencies, 1,450 wards and 890 post offices. It is a small Go program built on the [kenya-geo](https://github.com/rmiyoyo/kenya-geo) package, which carries all the data.

## Run it

```sh
go run .
```

Then open http://localhost:8080. It listens on `$PORT` when that is set, and `-addr` overrides both:

```sh
go run . -addr :3000
```

On Windows, ports 8077 to 8176 are often reserved for Hyper-V and WSL, so pick another port such as 3000 if 8080 is refused.

With Docker:

```sh
docker build -t mapping-kenya .
docker run -p 8080:8080 mapping-kenya
```

## Pages

| Page | Shows |
| --- | --- |
| `/` | a map of the counties and a table you can sort by population, area, density or voters |
| `/counties/32` | a county's wards on a map, its census figures, neighbours, constituencies and post offices |
| `/constituencies/Kibra` | a constituency's wards, highlighted on the county map |
| `/wards/0040` | one ward, its nearest post offices and the other wards in its constituency |
| `/postcodes/00100` | a post office, where it is and which ward it falls in |
| `/search?q=kibera` | fuzzy search across every kind of place |
| `/at?lat=-1.2884&lng=36.8233` | the ward, constituency and county for a point; click the map or use your location |
| `/addresses?q=P.O.+Box+90-40100+Eldoret` | whether an address's box, postal code and town agree |
| `/docs` | the API documentation |
| `/api/...` | the kenya-geo JSON API |

## API documentation

The `/docs` page isn't written here. It is [docs/API.md](https://github.com/rmiyoyo/kenya-geo/blob/main/docs/API.md) from the kenya-geo repository, fetched when the page is first opened and again at most once an hour, and rendered to match the rest of the site. The example URLs in it are rewritten to point at this site's `/api`. If GitHub can't be reached, the page keeps showing the last copy it had.

To read the docs from somewhere else, such as a branch or a local server, pass `-docs`:

```sh
go run . -docs https://raw.githubusercontent.com/rmiyoyo/kenya-geo/some-branch/docs/API.md
```

## How it works

The pages are plain HTML rendered on the server. The only JavaScript turns a click on the map, or your browser's location, into coordinates.

For anyone reading the code to learn Go:

- The templates, stylesheet and script live in `web/templates` and `web/static` and are compiled into the binary with `//go:embed`, so the server is one file with nothing to deploy beside it.
- `html/template` escapes everything it prints for where it appears: text, attributes and URLs each get the right escaping, so a search for `<script>` shows up as text. Each page is parsed together with `layout.html`, which defines the frame and shared pieces such as the map, and the page fills in its `content` block.
- The maps are SVG drawn in Go. `web/maps.go` projects longitude and latitude straight onto x and y (close enough this near the equator), drops points that land within a pixel of the previous one, and writes the result as SVG path data. All 48 maps are drawn once at startup, and a page copies the one it needs to highlight a ward or add a marker.
- Routes use the method and wildcard patterns `http.ServeMux` gained in Go 1.22, such as `GET /wards/{code}`, read back with `r.PathValue("code")`. The kenya-geo API is mounted under `/api` with `http.StripPrefix`.
- `web/docs.go` turns the Markdown into HTML with [goldmark](https://github.com/yuin/goldmark), the site's only dependency besides kenya-geo. It walks the parsed document to build the contents list from the headings. goldmark drops raw HTML by default, so nothing in the fetched file can inject a script. A `sync.Mutex` guards the cached copy, so many requests at once still fetch it only once.
- Each page renders into a buffer before anything is written, so a template error becomes a clean 500 instead of half a page.

## Checks

```sh
gofmt -l .
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
go test ./...
```

The tests render every county, constituency, ward and post office page, and serve a stand-in Markdown file to test the docs page.

## Licence

MIT. The data comes from the kenya-geo package; see its [sources](https://github.com/rmiyoyo/kenya-geo#sources) for licences and attribution.
