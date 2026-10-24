package engine

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// DASH (.mpd) download: pick the best video and audio Representation from
// the first Period, expand their segment URLs, download both tracks with
// runStream and merge them with ffmpeg.
//
// Supported addressing: SegmentTemplate with $Number$ or SegmentTimeline
// ($Time$), SegmentList, and single-file representations (BaseURL only).
// Encrypted (ContentProtection) and live (type="dynamic") manifests are
// refused.

type mpd struct {
	Type     string      `xml:"type,attr"`
	Duration string      `xml:"mediaPresentationDuration,attr"`
	BaseURL  []string    `xml:"BaseURL"`
	Periods  []mpdPeriod `xml:"Period"`
}

type mpdPeriod struct {
	Duration string        `xml:"duration,attr"`
	BaseURL  []string      `xml:"BaseURL"`
	Sets     []mpdAdaptSet `xml:"AdaptationSet"`
}

type mpdAdaptSet struct {
	ContentType string       `xml:"contentType,attr"`
	MimeType    string       `xml:"mimeType,attr"`
	BaseURL     []string     `xml:"BaseURL"`
	Protection  []struct{}   `xml:"ContentProtection"`
	Template    *mpdTemplate `xml:"SegmentTemplate"`
	List        *mpdList     `xml:"SegmentList"`
	Reps        []mpdRep     `xml:"Representation"`
}

type mpdRep struct {
	ID         string       `xml:"id,attr"`
	Bandwidth  int64        `xml:"bandwidth,attr"`
	Height     int          `xml:"height,attr"`
	MimeType   string       `xml:"mimeType,attr"`
	BaseURL    []string     `xml:"BaseURL"`
	Protection []struct{}   `xml:"ContentProtection"`
	Template   *mpdTemplate `xml:"SegmentTemplate"`
	List       *mpdList     `xml:"SegmentList"`
}

type mpdTemplate struct {
	Media          string       `xml:"media,attr"`
	Initialization string       `xml:"initialization,attr"`
	StartNumber    *int64       `xml:"startNumber,attr"`
	Timescale      *int64       `xml:"timescale,attr"`
	Duration       *int64       `xml:"duration,attr"`
	Timeline       *mpdTimeline `xml:"SegmentTimeline"`
}

type mpdTimeline struct {
	S []struct {
		T *int64 `xml:"t,attr"`
		D int64  `xml:"d,attr"`
		R int64  `xml:"r,attr"`
	} `xml:"S"`
}

type mpdList struct {
	Init *struct {
		SourceURL string `xml:"sourceURL,attr"`
		Range     string `xml:"range,attr"`
	} `xml:"Initialization"`
	Segments []struct {
		Media string `xml:"media,attr"`
		Range string `xml:"mediaRange,attr"`
	} `xml:"SegmentURL"`
}

// merge returns the representation-level template with attributes it
// doesn't set inherited from the adaptation set (DASH inheritance rules).
func merge(set, rep *mpdTemplate) *mpdTemplate {
	if rep == nil {
		return set
	}
	if set == nil {
		return rep
	}
	m := *rep
	if m.Media == "" {
		m.Media = set.Media
	}
	if m.Initialization == "" {
		m.Initialization = set.Initialization
	}
	if m.StartNumber == nil {
		m.StartNumber = set.StartNumber
	}
	if m.Timescale == nil {
		m.Timescale = set.Timescale
	}
	if m.Duration == nil {
		m.Duration = set.Duration
	}
	if m.Timeline == nil {
		m.Timeline = set.Timeline
	}
	return &m
}

var isoDur = regexp.MustCompile(`^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

// parseISODuration parses durations like "PT1H2M3.5S" into seconds.
func parseISODuration(s string) (float64, error) {
	m := isoDur.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || s == "P" || s == "PT" {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	secs := 0.0
	for i, mult := range []float64{86400, 3600, 60, 1} {
		if m[i+1] != "" {
			v, _ := strconv.ParseFloat(m[i+1], 64)
			secs += v * mult
		}
	}
	return secs, nil
}

type dashVarKind int

const (
	varRepID dashVarKind = iota
	varNumber
	varBandwidth
	varTime
)

type dashChunk struct {
	literal string
	isVar   bool
	kind    dashVarKind
	width   int
}

// dashTemplate represents a pre-parsed DASH URL template.
// Pre-parsing avoids expensive regular expression matching in high-frequency segment expansion loops.
type dashTemplate struct {
	chunks []dashChunk
}

var tmplVar = regexp.MustCompile(`\$(RepresentationID|Number|Bandwidth|Time)(%0(\d+)d)?\$`)

func parseDashTemplate(t string) *dashTemplate {
	matches := tmplVar.FindAllStringSubmatchIndex(t, -1)
	if len(matches) == 0 {
		return &dashTemplate{chunks: []dashChunk{{literal: strings.ReplaceAll(t, "$$", "$")}}}
	}
	chunks := make([]dashChunk, 0, len(matches)*2+1)
	lastIdx := 0
	for _, m := range matches {
		if m[0] > lastIdx {
			lit := strings.ReplaceAll(t[lastIdx:m[0]], "$$", "$")
			if lit != "" {
				chunks = append(chunks, dashChunk{literal: lit})
			}
		}
		varKindStr := t[m[2]:m[3]]
		var kind dashVarKind
		switch varKindStr {
		case "RepresentationID":
			kind = varRepID
		case "Number":
			kind = varNumber
		case "Bandwidth":
			kind = varBandwidth
		case "Time":
			kind = varTime
		}
		width := 0
		if m[6] != -1 && m[7] != -1 {
			width, _ = strconv.Atoi(t[m[6]:m[7]])
		}
		chunks = append(chunks, dashChunk{
			isVar: true,
			kind:  kind,
			width: width,
		})
		lastIdx = m[1]
	}
	if lastIdx < len(t) {
		lit := strings.ReplaceAll(t[lastIdx:], "$$", "$")
		if lit != "" {
			chunks = append(chunks, dashChunk{literal: lit})
		}
	}
	return &dashTemplate{chunks: chunks}
}

func (dt *dashTemplate) eval(rep *mpdRep, number, time int64) string {
	var b strings.Builder
	for _, c := range dt.chunks {
		if !c.isVar {
			b.WriteString(c.literal)
			continue
		}
		var val string
		switch c.kind {
		case varRepID:
			if rep != nil {
				val = rep.ID
			}
		case varNumber:
			val = strconv.FormatInt(number, 10)
		case varBandwidth:
			if rep != nil {
				val = strconv.FormatInt(rep.Bandwidth, 10)
			} else {
				val = "0"
			}
		case varTime:
			val = strconv.FormatInt(time, 10)
		}
		if c.width > len(val) {
			for i := 0; i < c.width-len(val); i++ {
				b.WriteByte('0')
			}
		}
		b.WriteString(val)
	}
	return b.String()
}

func fillTemplate(t string, rep *mpdRep, number, time int64) string {
	return parseDashTemplate(t).eval(rep, number, time)
}

// resolveChain applies BaseURL elements from outermost to innermost.
func resolveChain(base string, levels ...[]string) string {
	for _, l := range levels {
		if len(l) > 0 && strings.TrimSpace(l[0]) != "" {
			base = resolve(base, l[0])
		}
	}
	return base
}

func kindOf(set *mpdAdaptSet, rep *mpdRep) string {
	for _, s := range []string{set.ContentType, set.MimeType, rep.MimeType} {
		switch {
		case strings.HasPrefix(s, "video"):
			return "video"
		case strings.HasPrefix(s, "audio"):
			return "audio"
		case s != "":
			return "other"
		}
	}
	return ""
}

// segmentURLs expands a representation into its init + media segment URLs.
func segmentURLs(base string, set *mpdAdaptSet, rep *mpdRep, periodSecs float64) ([]string, error) {
	// Pre-parse base URL once outside the segment expansion loop to avoid url.Parse overhead.
	baseURL, _ := url.Parse(base)
	if l := rep.List; l != nil || set.List != nil {
		if l == nil {
			l = set.List
		}
		var urls []string
		if l.Init != nil {
			if l.Init.Range != "" {
				return nil, errors.New("byte-range DASH segments aren't supported yet")
			}
			urls = append(urls, resolveURL(baseURL, l.Init.SourceURL))
		}
		for _, s := range l.Segments {
			if s.Range != "" {
				return nil, errors.New("byte-range DASH segments aren't supported yet")
			}
			urls = append(urls, resolveURL(baseURL, s.Media))
		}
		return urls, nil
	}

	t := merge(set.Template, rep.Template)
	if t == nil || t.Media == "" { // single file: the BaseURL is the whole track
		return []string{base}, nil
	}
	start, scale := int64(1), int64(1)
	if t.StartNumber != nil {
		start = *t.StartNumber
	}
	if t.Timescale != nil && *t.Timescale > 0 {
		scale = *t.Timescale
	}
	var urls []string
	if t.Initialization != "" {
		initTpl := parseDashTemplate(t.Initialization)
		urls = append(urls, resolveURL(baseURL, initTpl.eval(rep, 0, 0)))
	}
	mediaTpl := parseDashTemplate(t.Media)
	switch {
	case t.Timeline != nil:
		n, time := start, int64(0)
		end := int64(periodSecs * float64(scale))
		for i, s := range t.Timeline.S {
			if s.T != nil {
				time = *s.T
			}
			repeat := s.R
			if repeat < 0 { // repeat until the next S or the end of the period
				until := end
				if i+1 < len(t.Timeline.S) && t.Timeline.S[i+1].T != nil {
					until = *t.Timeline.S[i+1].T
				}
				if s.D <= 0 || until <= time {
					return nil, errors.New("can't expand open-ended SegmentTimeline")
				}
				repeat = (until-time+s.D-1)/s.D - 1
			}
			for k := int64(0); k <= repeat; k++ {
				urls = append(urls, resolveURL(baseURL, mediaTpl.eval(rep, n, time)))
				n++
				time += s.D
			}
		}
	case t.Duration != nil && *t.Duration > 0:
		if periodSecs <= 0 {
			return nil, errors.New("manifest has no duration")
		}
		count := int64(math.Ceil(periodSecs * float64(scale) / float64(*t.Duration)))
		for k := int64(0); k < count; k++ {
			n := start + k
			urls = append(urls, resolveURL(baseURL, mediaTpl.eval(rep, n, k**t.Duration)))
		}
	default:
		return nil, errors.New("SegmentTemplate without duration or timeline")
	}
	if len(urls) > 20000 {
		return nil, errors.New("manifest lists too many segments")
	}
	return urls, nil
}

// loadDASH fetches and parses the manifest and returns the tracks to
// download: best video first, then best audio (either may be absent).
func (e *Engine) loadDASH(ctx context.Context, r Request) ([][]string, error) {
	text, err := e.fetchText(ctx, r, r.URL)
	if err != nil {
		return nil, err
	}
	var m mpd
	if err := xml.Unmarshal([]byte(text), &m); err != nil {
		return nil, fmt.Errorf("not a DASH manifest: %w", err)
	}
	if m.Type == "dynamic" {
		return nil, ErrLive
	}
	if len(m.Periods) == 0 {
		return nil, errors.New("manifest has no periods")
	}
	if len(m.Periods) > 1 {
		return nil, errors.New("multi-period DASH manifests aren't supported yet")
	}
	p := &m.Periods[0]
	secs := 0.0
	for _, s := range []string{p.Duration, m.Duration} {
		if s != "" {
			if v, err := parseISODuration(s); err == nil {
				secs = v
				break
			}
		}
	}

	type pick struct {
		set *mpdAdaptSet
		rep *mpdRep
	}
	maxH := maxFormatHeight(r.Format)
	best := map[string]pick{}
	fit := map[string]pick{}
	for si := range p.Sets {
		set := &p.Sets[si]
		for ri := range set.Reps {
			rep := &set.Reps[ri]
			k := kindOf(set, rep)
			if k != "video" && k != "audio" {
				continue
			}
			if len(set.Protection) > 0 || len(rep.Protection) > 0 {
				return nil, ErrEncrypted
			}
			if b, ok := best[k]; !ok || rep.Bandwidth > b.rep.Bandwidth {
				best[k] = pick{set, rep}
			}
			if k == "video" && maxH > 0 && rep.Height > 0 && rep.Height <= maxH {
				if f, ok := fit[k]; !ok || rep.Bandwidth > f.rep.Bandwidth {
					fit[k] = pick{set, rep}
				}
			}
		}
	}
	if f, ok := fit["video"]; ok {
		best["video"] = f
	}
	if r.Format == "audio" {
		if _, ok := best["audio"]; ok {
			delete(best, "video")
		}
	}
	var tracks [][]string
	for _, k := range []string{"video", "audio"} {
		b, ok := best[k]
		if !ok {
			continue
		}
		base := resolveChain(r.URL, m.BaseURL, p.BaseURL, b.set.BaseURL, b.rep.BaseURL)
		urls, err := segmentURLs(base, b.set, b.rep, secs)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, urls)
	}
	if len(tracks) == 0 {
		return nil, errors.New("manifest has no audio or video")
	}
	return tracks, nil
}

func (e *Engine) runDASH(ctx context.Context, d *Download) error {
	tracks, err := e.loadDASH(ctx, d.Req)
	if err != nil {
		return err
	}
	return e.runStream(ctx, d, tracks, ".mp4")
}
