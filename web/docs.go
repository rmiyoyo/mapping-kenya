package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const DefaultDocsURL = "https://raw.githubusercontent.com/rmiyoyo/kenya-geo/main/docs/API.md"

const docsTTL = time.Hour

const maxDocsSize = 1 << 20

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

type tocEntry struct {
	ID    string
	Title string
	Level int
}

type docsPage struct {
	Title   string
	HTML    template.HTML
	TOC     []tocEntry
	Fetched time.Time
}

type docs struct {
	url    string
	client *http.Client

	mu      sync.Mutex
	page    *docsPage
	checked time.Time
	err     error
}

func newDocs(url string) *docs {
	return &docs{url: url, client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *docs) get(ctx context.Context) (*docsPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.page != nil && time.Since(d.checked) < docsTTL {
		return d.page, nil
	}
	if d.page == nil && d.err != nil && time.Since(d.checked) < time.Minute {
		return nil, d.err
	}
	d.checked = time.Now()
	page, err := d.fetch(ctx)
	if err != nil {
		d.err = err
		return d.page, err
	}
	d.page, d.err = page, nil
	return page, nil
}

func (d *docs) fetch(ctx context.Context) (*docsPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: %s", d.url, resp.Status)
	}
	src, err := io.ReadAll(io.LimitReader(resp.Body, maxDocsSize+1))
	if err != nil {
		return nil, err
	}
	if len(src) > maxDocsSize {
		return nil, errors.New("the API document is larger than 1 MB")
	}
	return renderDocs(src)
}

func renderDocs(src []byte) (*docsPage, error) {
	doc := markdown.Parser().Parse(text.NewReader(src))
	page := &docsPage{Fetched: time.Now()}
	var first ast.Node
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		title := plainText(h, src)
		if h.Level == 1 && page.Title == "" {
			page.Title, first = title, h
			return ast.WalkSkipChildren, nil
		}
		if h.Level <= 3 {
			id, _ := h.AttributeString("id")
			idText, _ := id.([]byte)
			page.TOC = append(page.TOC, tocEntry{ID: string(idText), Title: strings.TrimPrefix(title, "GET "), Level: h.Level})
		}
		return ast.WalkSkipChildren, nil
	})
	if err != nil {
		return nil, err
	}
	if first != nil {
		doc.RemoveChild(doc, first)
	}
	var buf bytes.Buffer
	if err := markdown.Renderer().Render(&buf, src, doc); err != nil {
		return nil, err
	}
	page.HTML = template.HTML(buf.String())
	return page, nil
}

func plainText(n ast.Node, src []byte) string {
	var b bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
			continue
		}
		b.WriteString(plainText(c, src))
	}
	return b.String()
}
