package site

import (
	"bytes"
	"html"
	"html/template"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/rmiyoyo/kenya-geo/api"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const apiModule = "github.com/rmiyoyo/kenya-geo"

type docLink struct {
	ID     string
	Method string
	Path   string
	Title  string
}

type docSection struct {
	ID    string
	Title string
	Links []docLink
}

type apiDocs struct {
	Title    string
	Body     template.HTML
	Sections []docSection
	Version  string
	Source   string
}

type docsPage struct {
	*apiDocs
	Base string
}

func renderDocs(src []byte) (*apiDocs, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(docRenderer{}, 100))),
	)
	root := md.Parser().Parse(text.NewReader(src))

	d := &apiDocs{}
	for n := root.FirstChild(); n != nil; {
		next := n.NextSibling()
		if h, ok := n.(*ast.Heading); ok {
			title := plainText(h, src)
			id := headingID(h)
			switch h.Level {
			case 1:
				if d.Title == "" {
					d.Title = title
					root.RemoveChild(root, h)
				}
			case 2:
				d.Sections = append(d.Sections, docSection{ID: id, Title: title})
			case 3:
				if len(d.Sections) > 0 {
					l := docLink{ID: id, Title: title}
					l.Method, l.Path, _ = endpoint(title)
					s := &d.Sections[len(d.Sections)-1]
					s.Links = append(s.Links, l)
				}
			}
		}
		n = next
	}

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, root); err != nil {
		return nil, err
	}
	d.Body = template.HTML(buf.String())
	d.Version, d.Source = docsSource()
	return d, nil
}

func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func headingID(h *ast.Heading) string {
	if id, ok := h.AttributeString("id"); ok {
		if b, ok := id.([]byte); ok {
			return string(b)
		}
	}
	return ""
}

func endpoint(s string) (method, path string, ok bool) {
	method, path, ok = strings.Cut(s, " ")
	if !ok || method != strings.ToUpper(method) || !strings.HasPrefix(path, "/") || strings.Contains(path, " ") {
		return "", "", false
	}
	return method, path, true
}

var pseudoVersion = regexp.MustCompile(`-\d{14}-([0-9a-f]{12})$`)

func docsSource() (version, source string) {
	version, ref := "", "main"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path != apiModule {
				continue
			}
			if dep.Replace != nil {
				version = "from a local copy"
				break
			}
			version, ref = dep.Version, dep.Version
			if m := pseudoVersion.FindStringSubmatch(dep.Version); m != nil {
				ref = m[1]
			}
		}
	}
	return version, "https://" + apiModule + "/blob/" + ref + "/api/README.md"
}

type docRenderer struct{}

func (r docRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.heading)
	reg.Register(ast.KindFencedCodeBlock, r.code)
}

func (docRenderer) heading(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	h := n.(*ast.Heading)
	tag := "h" + string(rune('0'+h.Level))
	if !entering {
		w.WriteString("</" + tag + ">\n")
		return ast.WalkContinue, nil
	}
	id := headingID(h)
	w.WriteString("<" + tag + ` id="` + html.EscapeString(id) + `">`)
	if method, path, ok := endpoint(plainText(h, src)); ok && h.Level == 3 {
		w.WriteString(`<span class="method">` + html.EscapeString(method) + `</span> <code>` + html.EscapeString(path) + `</code>`)
		w.WriteString(` <a class="anchor" href="#` + html.EscapeString(id) + `" aria-label="Link to this endpoint">#</a>`)
		return ast.WalkSkipChildren, nil
	}
	return ast.WalkContinue, nil
}

func (docRenderer) code(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	c := n.(*ast.FencedCodeBlock)
	var body strings.Builder
	for i := 0; i < c.Lines().Len(); i++ {
		seg := c.Lines().At(i)
		body.Write(seg.Value(src))
	}
	code := strings.TrimRight(body.String(), "\r\n")
	lang := string(c.Language(src))

	if lang == "http" {
		if method, path, ok := endpoint(code); ok {
			w.WriteString(`<p class="request"><code><span class="method">` + html.EscapeString(method) + `</span> ` + html.EscapeString(path) + `</code>`)
			if method == http.MethodGet {
				w.WriteString(` <a class="try" href="/api` + html.EscapeString(path) + `">Try it</a>`)
			}
			w.WriteString("</p>\n")
			return ast.WalkSkipChildren, nil
		}
	}

	w.WriteString(`<pre class="code"><code>`)
	if lang == "json" {
		w.WriteString(highlightJSON(code))
	} else {
		w.WriteString(html.EscapeString(code))
	}
	w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

func highlightJSON(s string) string {
	var b strings.Builder
	span := func(class, tok string) {
		b.WriteString(`<span class="` + class + `">` + html.EscapeString(tok) + `</span>`)
	}
	for i := 0; i < len(s); {
		switch ch := s[i]; {
		case ch == '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(s))
			class := "s"
			if strings.HasPrefix(strings.TrimLeft(s[j:], " \t\r\n"), ":") {
				class = "k"
			}
			span(class, s[i:j])
			i = j
		case ch == '-' || ch >= '0' && ch <= '9':
			j := i + 1
			for j < len(s) && strings.IndexByte("0123456789.eE+-", s[j]) >= 0 {
				j++
			}
			span("n", s[i:j])
			i = j
		case strings.HasPrefix(s[i:], "true"), strings.HasPrefix(s[i:], "null"):
			span("b", s[i:i+4])
			i += 4
		case strings.HasPrefix(s[i:], "false"):
			span("b", s[i:i+5])
			i += 5
		default:
			b.WriteString(html.EscapeString(s[i : i+1]))
			i++
		}
	}
	return b.String()
}

func (s *site) docs(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	p := docsPage{apiDocs: s.apiDocs, Base: scheme + "://" + r.Host + "/api"}
	s.render(w, r, http.StatusOK, "docs", page{Title: "API", Data: p})
}

func mustRenderDocs() *apiDocs {
	d, err := renderDocs([]byte(api.Docs))
	if err != nil {
		panic(err)
	}
	return d
}
