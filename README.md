# mapping-kenya

A website for Kenya's counties, constituencies, wards and post offices, built on the [kenya-geo](https://github.com/rmiyoyo/kenya-geo) Go package.

## Run it

```sh
go run .
```

Then open http://localhost:8080. It listens on `$PORT` when that is set, and `-addr` overrides both:

```sh
go run . -addr :3000
```

On Windows, Hyper-V and WSL often reserve a block of ports around 8080, which shows up as "An attempt was made to access a socket in a way forbidden by its access permissions". Pick another port with `-addr`; `netsh interface ipv4 show excludedportrange protocol=tcp` lists the reserved ones.

With Docker:

```sh
docker build -t mapping-kenya .
docker run -p 8080:8080 mapping-kenya
```

## Pages

| Page | Shows |
| --- | --- |
| `/` | a map of the 47 counties, shaded by population, density, voters or area, and a table you can sort by each |
| `/counties/32` | a county's wards on a map shaded by registered voters, a chart of voters by constituency, its census figures, neighbours and post offices |
| `/constituencies/Kibra` | a constituency's wards, highlighted on the county map |
| `/wards/0040` | one ward, its nearest post offices and the other wards in its constituency |
| `/postcodes/00100` | a post office, where it is and which ward it falls in |
| `/search?q=kibera` | fuzzy search across every kind of place |
| `/at?lat=-1.2884&lng=36.8233` | the ward, constituency and county for a point; click the map or use your location |
| `/addresses?q=P.O.+Box+90-40100+Eldoret` | whether an address's box, postal code and town agree |
| `/api/` | the API documentation |
| `/api/...` | the JSON API from kenya-geo's `api` package |

The pages are plain HTML rendered on the server, and they work without JavaScript. [D3](https://d3js.org) then makes the maps interactive:

- Every map zooms and pans: drag, pinch, use the + and − buttons, or hold Ctrl and scroll. Ward and constituency pages start zoomed in on the place.
- Pointing at a county or ward shows its figures in a tooltip.
- The home map is shaded by population, density, registered voters or area, with a switch and a legend. On the home page, pointing at a county also highlights its row in the table, and the other way round.
- County maps are shaded by registered voters per ward. Below them, each constituency has a bar for its voters and opens to list its wards. Pointing at a constituency highlights its wards on the map, and pointing at a ward highlights its constituency.
- Lake Victoria and Lake Turkana are drawn on the maps from Natural Earth's public-domain lake outlines, kept in `internal/site/lakes.json`.
- The home page table shows the top ten counties for the chosen column, with a link to show all 47.

## The API documentation

The API docs are written once, in kenya-geo's [api/README.md](https://github.com/rmiyoyo/kenya-geo/blob/main/api/README.md), which reads well on GitHub. kenya-geo embeds that file and exports it as `api.Docs`, and this site renders it at `/api/` in the site's own style.

Because the Markdown comes through the Go module rather than over the network, the docs always describe the exact version of the API this site serves. Update both by bumping the dependency:

```sh
go get github.com/rmiyoyo/kenya-geo@latest
```

The page shows which kenya-geo version it came from and links to that version of the file on GitHub.

To work on the docs and the site together, point the module at a local checkout of kenya-geo next to this one, and remove the line before committing:

```sh
go mod edit -replace github.com/rmiyoyo/kenya-geo=../kenya-geo
go mod edit -dropreplace github.com/rmiyoyo/kenya-geo
```

The docs follow a few conventions that the site turns into more than plain Markdown:

- `## Heading` starts a section of the contents sidebar, and each `### Heading` under it becomes an entry.
- A heading such as `### GET /counties/{county}` is an endpoint: it gets a method badge and a link to itself.
- A fenced block marked `http` holding a single request, such as `GET /counties/32`, becomes a request bar with a *Try it* link to `/api/counties/32` on this site.
- A fenced block marked `json` is coloured: keys, strings, numbers and literals.

## How it is put together

For anyone reading the code to learn Go:

- `main.go` sets up an `http.Server` with timeouts, logs each request, and shuts down cleanly on Ctrl-C or SIGTERM: `signal.NotifyContext` gives a context that is cancelled by the signal, and `Shutdown` stops taking new connections and waits for the requests in flight.
- The code lives in `internal/site`. Go won't let another module import a package under `internal/`, so the site's handlers stay private to this repo while kenya-geo stays the public library.
- The templates, stylesheet and script live in `internal/site/templates` and `internal/site/static` and are compiled into the binary with `//go:embed`, so the server is one file with nothing to deploy beside it. kenya-geo uses the same trick for `api/README.md`: a `//go:embed` on a `string` variable turns a file into a constant-like value other modules can import.
- `html/template` escapes everything it prints for where it appears: text, attributes and URLs each get the right escaping, so a search for `<script>` shows up as text. Each page is parsed together with `layout.html`, which defines the frame and shared pieces such as the map, and the page fills in its `content` block. The rendered docs are the one exception: they are trusted HTML made from Markdown at startup, so they are passed in as `template.HTML`.
- `internal/site/docs.go` renders the Markdown with [goldmark](https://github.com/yuin/goldmark). Goldmark first parses the text into a tree of nodes (`ast.Heading`, `ast.FencedCodeBlock` and so on). The site walks the top of that tree to build the contents sidebar, then renders it with a small custom `NodeRenderer` that takes over headings and code blocks and leaves everything else, such as tables and links, to goldmark's own HTML renderer. Registering it with a higher priority is how goldmark lets you replace part of its output without forking it.
- `runtime/debug.ReadBuildInfo` reports the versions of the modules compiled into the binary, which is how the docs page knows which kenya-geo version it is showing.
- The maps are SVG drawn in Go. `internal/site/maps.go` projects longitude and latitude straight onto x and y (close enough this near the equator), drops points that land within a pixel of the previous one, and writes the result as SVG path data. All 48 maps are drawn once at startup, and a page copies the one it needs to highlight a ward or add a marker.
- Routes use the method and wildcard patterns `http.ServeMux` gained in Go 1.22, such as `GET /wards/{code}`, read back with `r.PathValue("code")`. `GET /api/{$}` matches only `/api/` itself, so it can serve the docs while the broader `/api/` pattern hands everything below it to kenya-geo's API handler through `http.StripPrefix`.
- Each page renders into a buffer before anything is written, so a template error becomes a clean 500 instead of half a page.
- The D3 code in `internal/site/static/charts.js` is progressive enhancement: it never fetches data. The server writes each county's and ward's figures onto its SVG path as `data-` attributes (`data-population`, `data-voters` and so on). D3 reads those, then uses `d3.scaleQuantile` to split the values into five equal-sized colour bands taken from the `--seq-1` to `--seq-5` CSS variables, and `d3.zoom` to move a `<g>` that wraps the shapes. The constituency bars need no JavaScript at all: the server works out each one's share of the largest and writes it as a CSS width. If D3 fails to load, the plain maps still work.
- D3 7.9.0 loads from jsDelivr with a Subresource Integrity hash, so the browser refuses the file if the CDN ever serves different bytes.

## Tests

```sh
go test ./...
```

The tests render every county, constituency, ward and post office page, check that every link in the docs' contents points at a heading that exists, and follow every *Try it* link to make sure it returns 200.

## Licence

MIT. The data's sources and licences are listed in [kenya-geo](https://github.com/rmiyoyo/kenya-geo#sources).
