package main

import (
	"strings"
	"testing"
)

func TestParseVolumePercent(t *testing.T) {
	cases := map[string]float64{
		"100": 1.0,
		"150": 1.5,
		"50":  0.5,
		"":    1.0,
		"abc": 1.0,
		"5":   1.0, // below the allowed range
		"900": 1.0, // above the allowed range
	}

	for in, want := range cases {
		if got := parseVolumePercent(in); got != want {
			t.Errorf("parseVolumePercent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestBuildAudioFilterNoSource(t *testing.T) {
	graph, maps := buildAudioFilter(audioMixOptions{MicIndex: -1, SysIndex: -1})
	if graph != "" || maps != nil {
		t.Fatalf("expected an empty graph, got %q %v", graph, maps)
	}
}

func TestBuildAudioFilterMicOnly(t *testing.T) {
	graph, maps := buildAudioFilter(audioMixOptions{
		MicIndex: 0, SysIndex: -1, MicVolume: 1.5, Denoise: true,
	})

	for _, want := range []string{"[0:a:0]", "highpass=f=80", "afftdn", "volume=1.50", "[aout]"} {
		if !strings.Contains(graph, want) {
			t.Errorf("graph %q does not contain %q", graph, want)
		}
	}

	if len(maps) != 1 || maps[0] != "[aout]" {
		t.Errorf("unexpected maps %v", maps)
	}
}

func TestBuildAudioFilterSystemOnlyIgnoresDenoise(t *testing.T) {
	graph, _ := buildAudioFilter(audioMixOptions{
		MicIndex: -1, SysIndex: 0, SysVolume: 1.0, Denoise: true,
	})

	if strings.Contains(graph, "afftdn") {
		t.Errorf("denoise must only apply to the microphone: %q", graph)
	}

	if !strings.Contains(graph, "anull") {
		t.Errorf("unchanged audio should use anull: %q", graph)
	}
}

func TestBuildAudioFilterMixed(t *testing.T) {
	graph, maps := buildAudioFilter(audioMixOptions{
		MicIndex: 0, SysIndex: 1, MicVolume: 1.0, SysVolume: 0.5,
	})

	for _, want := range []string{"amix=inputs=2", "[1:a:0]volume=0.50", "alimiter", "[aout]"} {
		if !strings.Contains(graph, want) {
			t.Errorf("graph %q does not contain %q", graph, want)
		}
	}

	if len(maps) != 1 {
		t.Errorf("mixed audio must produce one track, got %v", maps)
	}
}

func TestBuildAudioFilterSeparate(t *testing.T) {
	graph, maps := buildAudioFilter(audioMixOptions{
		MicIndex: 0, SysIndex: 1, MicVolume: 1.0, SysVolume: 1.0, Separate: true,
	})

	if strings.Contains(graph, "amix") {
		t.Errorf("separate tracks must not be mixed: %q", graph)
	}

	if len(maps) != 2 || maps[0] != "[a0]" || maps[1] != "[a1]" {
		t.Errorf("unexpected maps %v", maps)
	}
}

func TestSeparateIgnoredWithOneSource(t *testing.T) {
	_, maps := buildAudioFilter(audioMixOptions{
		MicIndex: 0, SysIndex: -1, MicVolume: 1.0, Separate: true,
	})

	if len(maps) != 1 {
		t.Errorf("one source can only give one track, got %v", maps)
	}
}
