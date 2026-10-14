package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseISODuration(t *testing.T) {
	for in, want := range map[string]float64{"PT10S": 10, "PT1H2M3.5S": 3723.5, "PT0.5S": 0.5, "P1DT1S": 86401, "PT2M": 120} {
		if got, err := parseISODuration(in); err != nil || got != want {
			t.Errorf("%s: got %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "10s", "PT", "P"} {
		if _, err := parseISODuration(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func dashServer(t *testing.T, manifest string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, manifest)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v/manifest.mpd"
}

func load(t *testing.T, manifest string) ([][]string, error) {
	url := dashServer(t, manifest)
	return New(t.TempDir()).loadDASH(context.Background(), Request{URL: url})
}

const numberMPD = `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT9.5S">
 <Period>
  <AdaptationSet contentType="video">
   <SegmentTemplate media="$RepresentationID$/seg-$Number%03d$.m4s" initialization="$RepresentationID$/init.mp4" startNumber="1" timescale="1000" duration="4000"/>
   <Representation id="360p" bandwidth="500000" height="360"/>
   <Representation id="1080p" bandwidth="5000000" height="1080"/>
  </AdaptationSet>
  <AdaptationSet mimeType="audio/mp4">
   <Representation id="aud" bandwidth="128000">
    <SegmentTemplate media="a/$Number$.m4s" initialization="a/init.mp4" duration="4"/>
   </Representation>
  </AdaptationSet>
  <AdaptationSet contentType="text"><Representation id="subs" bandwidth="1"/></AdaptationSet>
 </Period>
</MPD>`

func TestDASHNumberTemplate(t *testing.T) {
	tracks, err := load(t, numberMPD)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("want video+audio, got %d tracks", len(tracks))
	}
	v, a := tracks[0], tracks[1]
	// 9.5s / 4s = 3 segments (rounded up) + init
	if len(v) != 4 || !strings.HasSuffix(v[0], "/v/1080p/init.mp4") || !strings.HasSuffix(v[3], "/v/1080p/seg-003.m4s") {
		t.Fatalf("video urls: %v", v)
	}
	if len(a) != 4 || !strings.HasSuffix(a[1], "/v/a/1.m4s") {
		t.Fatalf("audio urls: %v", a)
	}
}

func TestDASHTimeline(t *testing.T) {
	tracks, err := load(t, `<MPD type="static" mediaPresentationDuration="PT10S"><Period>
	 <AdaptationSet contentType="video"><SegmentTemplate media="v_$Time$.m4s" initialization="v_init.mp4" timescale="10">
	  <SegmentTimeline><S t="0" d="20" r="2"/><S d="15"/><S d="10" r="-1"/></SegmentTimeline></SegmentTemplate>
	  <Representation id="v" bandwidth="1"/></AdaptationSet></Period></MPD>`)
	if err != nil {
		t.Fatal(err)
	}
	// times 0,20,40 (r=2), 60 (d=15), then 75,85,95 until the 10s end (r=-1)
	want := []string{"/v_init.mp4", "/v_0.m4s", "/v_20.m4s", "/v_40.m4s", "/v_60.m4s", "/v_75.m4s", "/v_85.m4s", "/v_95.m4s"}
	if len(tracks[0]) != len(want) {
		t.Fatalf("got %d urls: %v", len(tracks[0]), tracks[0])
	}
	for i, suffix := range want {
		if !strings.HasSuffix(tracks[0][i], suffix) {
			t.Fatalf("url %d = %s, want ...%s", i, tracks[0][i], suffix)
		}
	}
}

func TestDASHRejects(t *testing.T) {
	enc := `<MPD type="static" mediaPresentationDuration="PT4S"><Period><AdaptationSet contentType="video">
	  <ContentProtection schemeIdUri="urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed"/>
	  <Representation id="v" bandwidth="1"><BaseURL>v.mp4</BaseURL></Representation></AdaptationSet></Period></MPD>`
	live := `<MPD type="dynamic"><Period><AdaptationSet contentType="video"><Representation id="v" bandwidth="1"/></AdaptationSet></Period></MPD>`
	if _, err := load(t, enc); !errors.Is(err, ErrEncrypted) {
		t.Fatalf("encrypted: %v", err)
	}
	if _, err := load(t, live); !errors.Is(err, ErrLive) {
		t.Fatalf("live: %v", err)
	}
}

func TestDASHSingleFile(t *testing.T) {
	tracks, err := load(t, `<MPD type="static" mediaPresentationDuration="PT4S"><BaseURL>https://cdn.example/media/</BaseURL><Period>
	 <AdaptationSet mimeType="video/mp4"><Representation id="v" bandwidth="9"><BaseURL>video_1080.mp4</BaseURL></Representation></AdaptationSet>
	</Period></MPD>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || len(tracks[0]) != 1 || tracks[0][0] != "https://cdn.example/media/video_1080.mp4" {
		t.Fatalf("got %v", tracks)
	}
}
