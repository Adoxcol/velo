// changelog turns CHANGELOG.md into the website's changelog page. Run from
// the repo root after editing CHANGELOG.md:
//
//	go run ./tools/changelog
//
// It understands the small Markdown subset CHANGELOG.md uses: "## version —
// date" headings, "### group" headings, "- " bullets, paragraphs, **bold**
// and `code`.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"html"
	"html/template"
	"log"
	"os"
	"regexp"
	"strings"
)

type group struct {
	Title string
	Items []template.HTML
}

type release struct {
	Version, Date string
	Intro         []template.HTML
	Groups        []*group
}

var (
	boldRe = regexp.MustCompile(`\*\*(.+?)\*\*`)
	codeRe = regexp.MustCompile("`([^`]+)`")
)

// inline escapes text, then applies **bold** and `code`.
func inline(s string) template.HTML {
	s = html.EscapeString(s)
	s = codeRe.ReplaceAllString(s, "<code>$1</code>")
	s = boldRe.ReplaceAllString(s, "<b>$1</b>")
	return template.HTML(s)
}

func parse(path string) ([]*release, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []*release
	var cur *release
	var g *group
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "## "):
			v, d, _ := strings.Cut(strings.TrimPrefix(line, "## "), "—")
			cur, g = &release{Version: strings.TrimSpace(v), Date: strings.TrimSpace(d)}, nil
			out = append(out, cur)
		case cur == nil || line == "":
		case strings.HasPrefix(line, "### "):
			g = &group{Title: strings.TrimPrefix(line, "### ")}
			cur.Groups = append(cur.Groups, g)
		case strings.HasPrefix(line, "- "):
			if g == nil {
				g = &group{}
				cur.Groups = append(cur.Groups, g)
			}
			g.Items = append(g.Items, inline(strings.TrimPrefix(line, "- ")))
		default:
			cur.Intro = append(cur.Intro, inline(line))
		}
	}
	return out, sc.Err()
}

func main() {
	in := flag.String("in", "CHANGELOG.md", "changelog to read")
	out := flag.String("out", "site/changelog.html", "page to write")
	flag.Parse()
	releases, err := parse(*in)
	if err != nil {
		log.Fatal(err)
	}
	var b bytes.Buffer
	if err := page.Execute(&b, releases); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d releases)", *out, len(releases))
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Velo changelog</title>
<meta name="description" content="What's new in each version of Velo, the free and open-source download manager for Windows.">
<meta name="theme-color" content="#0d1728">
<link rel="icon" href="favicon.ico" sizes="any">
<!-- Generated from CHANGELOG.md by tools/changelog. Edit that file, not this one. -->
<style>
  :root { --primary: #1677ff; --navy: #0d1728; --bg: #ffffff; --surface: #ffffff; --ink: #0d1728; --muted: #64748b; --line: #e6ebf2; --soft: #eef5ff; --ok: #22c55e; }
  @media (prefers-color-scheme: dark) { :root { --bg: #0a1120; --surface: #111b2e; --ink: #edf2f9; --muted: #97a6ba; --line: #1d2a41; --soft: #12233f; } }
  * { box-sizing: border-box; }
  body { margin: 0; background: var(--bg); color: var(--ink); font: 16px/1.65 "Segoe UI Variable Text", "Segoe UI", system-ui, -apple-system, sans-serif; -webkit-font-smoothing: antialiased; }
  a { color: inherit; }
  .wrap { max-width: 1120px; margin: 0 auto; padding: 0 24px; }
  header { background: var(--navy); color: #fff; position: relative; overflow: hidden; }
  header::before { content: ""; position: absolute; inset: 0; background: radial-gradient(60% 90% at 50% 0%, rgba(22, 119, 255, .4), transparent 70%); }
  header .wrap { position: relative; }
  nav { display: flex; align-items: center; gap: 26px; height: 72px; }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 21px; text-decoration: none; color: #fff; }
  .brand svg { width: 22px; height: 22px; }
  nav .links { display: flex; gap: 26px; margin-left: auto; font-size: 15px; align-items: center; }
  nav .links a { text-decoration: none; color: #b8c4d6; }
  nav .links a:hover { color: #fff; }
  .btn { display: inline-flex; padding: 9px 16px; border-radius: 11px; background: var(--primary); color: #fff !important; font-weight: 600; font-size: 14.5px; text-decoration: none; }
  .intro { padding: 56px 0 64px; }
  h1 { font-size: clamp(36px, 6vw, 56px); line-height: 1.05; letter-spacing: -.03em; margin: 0 0 12px; }
  .intro p { color: #aebbd0; font-size: 18px; margin: 0; max-width: 560px; }
  main { padding: 64px 0 40px; }
  .rel { display: grid; grid-template-columns: 220px 1fr; gap: 40px; padding: 0 0 56px; }
  .rel + .rel { border-top: 1px solid var(--line); padding-top: 56px; }
  .ver { position: sticky; top: 24px; align-self: start; }
  .ver b { display: block; font-size: 26px; letter-spacing: -.02em; }
  .ver span { color: var(--muted); font-size: 14.5px; }
  .ver .latest { display: inline-block; margin-top: 10px; padding: 3px 10px; border-radius: 99px; background: var(--soft); color: var(--primary); font-size: 12.5px; font-weight: 600; }
  .body > p { margin: 0 0 16px; color: var(--muted); font-size: 16.5px; }
  h3 { font-size: 13px; text-transform: uppercase; letter-spacing: .08em; color: var(--primary); margin: 0 0 12px; }
  h3 ~ h3, ul + h3 { margin-top: 28px; }
  ul { margin: 0; padding: 0; list-style: none; display: grid; gap: 12px; }
  li { position: relative; padding-left: 20px; color: var(--muted); }
  li::before { content: ""; position: absolute; left: 2px; top: .7em; width: 6px; height: 6px; border-radius: 50%; background: var(--primary); opacity: .7; }
  li b { color: var(--ink); font-weight: 600; }
  code { font: 13.5px ui-monospace, "Cascadia Code", Consolas, monospace; background: var(--soft); padding: 1px 6px; border-radius: 5px; color: var(--ink); }
  footer { border-top: 1px solid var(--line); padding: 36px 0 56px; color: var(--muted); font-size: 14px; }
  footer .wrap { display: flex; flex-wrap: wrap; gap: 16px 28px; }
  footer .links { display: flex; gap: 22px; margin-left: auto; flex-wrap: wrap; }
  footer a { text-decoration: none; }
  footer a:hover { color: var(--ink); }
  @media (max-width: 760px) {
    .wrap { padding: 0 16px; }
    nav .links a:not(.btn) { display: none; }
    .rel { grid-template-columns: 1fr; gap: 16px; }
    .ver { position: static; }
    footer .links { margin-left: 0; }
  }
</style>
</head>
<body>
<header>
  <div class="wrap">
    <nav>
      <a class="brand" href="/"><svg viewBox="0 0 24 24"><path d="M13.5 2 4 13.5h6.5L9 22l11-12.5h-6.8z" fill="#1677ff"/></svg>velo.</a>
      <div class="links">
        <a href="/#features">Features</a>
        <a href="/changelog">Changelog</a>
        <a href="https://github.com/Adoxcol/velo">GitHub</a>
        <a class="btn" href="/#download">Download</a>
      </div>
    </nav>
    <div class="intro">
      <h1>Changelog</h1>
      <p>What's new in each version of Velo.</p>
    </div>
  </div>
</header>
<main>
  <div class="wrap">
{{- range $i, $r := .}}
    <article class="rel" id="v{{$r.Version}}">
      <div class="ver"><b>{{$r.Version}}</b><span>{{$r.Date}}</span>{{if eq $i 0}}<br><span class="latest">Latest</span>{{end}}</div>
      <div class="body">
        {{- range $r.Intro}}<p>{{.}}</p>{{end}}
        {{- range $r.Groups}}
        {{if .Title}}<h3>{{.Title}}</h3>{{end}}
        <ul>{{range .Items}}
          <li>{{.}}</li>{{end}}
        </ul>
        {{- end}}
      </div>
    </article>
{{- end}}
  </div>
</main>
<footer>
  <div class="wrap">
    <span>Velo · Open source · MIT license · Made by <a href="https://www.adoxcol.com">Adoxcol</a></span>
    <div class="links">
      <a href="https://github.com/Adoxcol/velo">GitHub</a>
      <a href="https://github.com/Adoxcol/velo/issues">Report a problem</a>
      <a href="https://studio.adoxcol.com">Studio music player</a>
      <a href="https://www.adoxcol.com">adoxcol.com</a>
      <a href="https://gist.github.com/Adoxcol/ea9009ab50bad69a5067c20ae35edd49">Privacy</a>
    </div>
  </div>
</footer>
</body>
</html>
`))
