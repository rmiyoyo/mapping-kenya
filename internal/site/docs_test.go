package site

import (
	"regexp"
	"strings"
	"testing"
)

func TestDocsLinks(t *testing.T) {
	_, body := get(t, "/api/")
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="([^"]+)"`).FindAllStringSubmatch(body, -1) {
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(body, -1) {
		if !ids[m[1]] {
			t.Errorf("link to #%s, but no element has that id", m[1])
		}
	}
	tries := regexp.MustCompile(`class="try" href="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(tries) < 20 {
		t.Errorf("%d Try it links, want one per endpoint", len(tries))
	}
	for _, m := range tries {
		path := strings.ReplaceAll(m[1], "&amp;", "&")
		if status, _ := get(t, path); status != 200 {
			t.Errorf("Try it %s: status %d", path, status)
		}
	}
}

func TestDocsSections(t *testing.T) {
	d := mustRenderDocs()
	if d.Title != "HTTP API" {
		t.Errorf("title %q", d.Title)
	}
	endpoints := 0
	for _, s := range d.Sections {
		for _, l := range s.Links {
			if l.Method != "" {
				endpoints++
			}
		}
	}
	if endpoints < 20 {
		t.Errorf("%d endpoints in the contents, want at least 20", endpoints)
	}
}

func TestHighlightJSON(t *testing.T) {
	got := highlightJSON(`{"a": "b\"c", "n": -1.5e3, "ok": true, "x": null}`)
	for _, want := range []string{
		`<span class="k">&#34;a&#34;</span>`,
		`<span class="s">&#34;b\&#34;c&#34;</span>`,
		`<span class="n">-1.5e3</span>`,
		`<span class="b">true</span>`,
		`<span class="b">null</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("highlightJSON lacks %s in %s", want, got)
		}
	}
}

func TestEndpoint(t *testing.T) {
	for in, want := range map[string]bool{
		"GET /counties":       true,
		"GET /at?lat=1&lng=2": true,
		"Base URL":            false,
		"Get /counties":       false,
		"GET counties":        false,
		"GET /a b":            false,
	} {
		if _, _, ok := endpoint(in); ok != want {
			t.Errorf("endpoint(%q) ok = %v, want %v", in, ok, want)
		}
	}
}

func TestSourceURL(t *testing.T) {
	for version, ref := range map[string]string{
		"v0.3.0":                               "v0.3.0",
		"main":                                 "main",
		"v0.2.1-0.20261011014517-f73b1d7a1074": "f73b1d7a1074",
		"v0.0.0-20261011014517-f73b1d7a1074":   "f73b1d7a1074",
		"v0.3.0-rc.1.0.20261011014517-f73b1d7a1074": "f73b1d7a1074",
	} {
		want := "https://github.com/rmiyoyo/kenya-geo/blob/" + ref + "/api/README.md"
		if got := sourceURL(version); got != want {
			t.Errorf("sourceURL(%q) = %q, want %q", version, got, want)
		}
	}
}
