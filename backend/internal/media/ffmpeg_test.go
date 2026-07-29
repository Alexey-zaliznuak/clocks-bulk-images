package media

import (
	"strings"
	"testing"
)

func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestStretchArgsSlowsClipToSoundtrack(t *testing.T) {
	f := New(Options{OutputFPS: 30, MaxStretchFactor: 6})

	args, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 8)
	if err != nil {
		t.Fatalf("stretchArgs: %v", err)
	}
	// A 4s clip over an 8s track must be slowed exactly 2x…
	if filter := argValue(args, "-filter_complex"); !strings.Contains(filter, "setpts=PTS*2.000000") {
		t.Fatalf("expected a 2x slowdown, got filter %q", filter)
	}
	// …and the output must be cut to the soundtrack length.
	if got := argValue(args, "-t"); got != "8.000" {
		t.Fatalf("-t = %q, want 8.000", got)
	}
	if filter := argValue(args, "-filter_complex"); !strings.Contains(filter, "fps=30") {
		t.Fatalf("expected fps=30 in filter %q", filter)
	}
	if got := argValue(args, "-map"); got != "[v]" {
		t.Fatalf("first -map = %q, want [v] (the retimed video)", got)
	}
}

func TestStretchArgsSpeedsUpWhenAudioIsShorter(t *testing.T) {
	f := New(Options{OutputFPS: 30, MaxStretchFactor: 6})

	args, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 2)
	if err != nil {
		t.Fatalf("stretchArgs: %v", err)
	}
	if filter := argValue(args, "-filter_complex"); !strings.Contains(filter, "setpts=PTS*0.500000") {
		t.Fatalf("expected a 2x speedup, got filter %q", filter)
	}
	if got := argValue(args, "-t"); got != "2.000" {
		t.Fatalf("-t = %q, want 2.000", got)
	}
}

func TestStretchArgsRejectsExcessiveSlowdown(t *testing.T) {
	f := New(Options{OutputFPS: 30, MaxStretchFactor: 6})

	if _, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 60); err == nil {
		t.Fatal("expected 15x slowdown to be rejected")
	}
	// Exactly at the limit is still allowed.
	if _, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 24); err != nil {
		t.Fatalf("6x slowdown should be allowed, got %v", err)
	}
}

func TestStretchArgsRejectsUnknownDurations(t *testing.T) {
	f := New(Options{})
	if _, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 0, 8); err == nil {
		t.Fatal("expected an error for a zero-length video")
	}
	if _, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 0); err == nil {
		t.Fatal("expected an error for a zero-length soundtrack")
	}
}

func TestSmoothStretchUsesInterpolation(t *testing.T) {
	f := New(Options{OutputFPS: 30, MaxStretchFactor: 6, SmoothStretch: true})

	args, err := f.stretchArgs("in.mp4", "song.mp3", "out.mp4", 4, 8)
	if err != nil {
		t.Fatalf("stretchArgs: %v", err)
	}
	if filter := argValue(args, "-filter_complex"); !strings.Contains(filter, "minterpolate") {
		t.Fatalf("expected minterpolate in filter %q", filter)
	}
}
