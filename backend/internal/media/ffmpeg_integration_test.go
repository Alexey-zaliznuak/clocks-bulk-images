package media

import (
	"context"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// requireFFmpeg skips the test when the binaries are not installed, so the suite
// still runs on machines without ffmpeg.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

// makeSilentClip renders a synthetic video without an audio track.
func makeSilentClip(t *testing.T, path string, seconds int) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=24:duration="+itoa(seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v: %s", err, out)
	}
}

// makeTone renders a synthetic mp3 of the given length.
func makeTone(t *testing.T, path string, seconds int) {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-nostdin", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+itoa(seconds),
		"-c:a", "libmp3lame", "-q:a", "5",
		path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make tone: %v: %s", err, out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// The core promise of the mixing stage: a 4s silent clip and an 8s track come out
// as a single 8s video with sound.
func TestStretchAndMuxMatchesSoundtrackLength(t *testing.T) {
	requireFFmpeg(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	song := filepath.Join(dir, "song.mp3")
	out := filepath.Join(dir, "out.mp4")
	makeSilentClip(t, clip, 4)
	makeTone(t, song, 8)

	f := New(Options{Concurrency: 1, OutputFPS: 30, MaxStretchFactor: 6, TempDir: dir})
	if err := f.StretchAndMux(ctx, clip, song, out); err != nil {
		t.Fatalf("StretchAndMux: %v", err)
	}

	info, err := f.Probe(ctx, out)
	if err != nil {
		t.Fatalf("probe result: %v", err)
	}
	if !info.HasVideo || !info.HasAudio {
		t.Fatalf("result should carry both streams, got video=%v audio=%v", info.HasVideo, info.HasAudio)
	}
	// Container timestamps land a few dozen milliseconds off; anything larger
	// would be audible drift against the music.
	if math.Abs(info.Duration-8) > 0.25 {
		t.Fatalf("result is %.3fs, expected ~8s", info.Duration)
	}
}

// A track shorter than the clip speeds the footage up instead.
func TestStretchAndMuxSpeedsUpForShortTrack(t *testing.T) {
	requireFFmpeg(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	song := filepath.Join(dir, "song.mp3")
	out := filepath.Join(dir, "out.mp4")
	makeSilentClip(t, clip, 4)
	makeTone(t, song, 2)

	f := New(Options{Concurrency: 1, OutputFPS: 30, MaxStretchFactor: 6, TempDir: dir})
	if err := f.StretchAndMux(ctx, clip, song, out); err != nil {
		t.Fatalf("StretchAndMux: %v", err)
	}
	info, err := f.Probe(ctx, out)
	if err != nil {
		t.Fatalf("probe result: %v", err)
	}
	if math.Abs(info.Duration-2) > 0.25 {
		t.Fatalf("result is %.3fs, expected ~2s", info.Duration)
	}
}

func TestExtractMP3(t *testing.T) {
	requireFFmpeg(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	song := filepath.Join(dir, "song.mp3")
	withSound := filepath.Join(dir, "with-sound.mp4")
	out := filepath.Join(dir, "out.mp3")
	makeSilentClip(t, clip, 3)
	makeTone(t, song, 3)

	f := New(Options{Concurrency: 1, OutputFPS: 30, MaxStretchFactor: 6, TempDir: dir})
	if err := f.StretchAndMux(ctx, clip, song, withSound); err != nil {
		t.Fatalf("prepare video with sound: %v", err)
	}

	if err := f.ExtractMP3(ctx, withSound, out); err != nil {
		t.Fatalf("ExtractMP3: %v", err)
	}
	info, err := f.Probe(ctx, out)
	if err != nil {
		t.Fatalf("probe mp3: %v", err)
	}
	if !info.HasAudio || info.HasVideo {
		t.Fatalf("expected audio only, got video=%v audio=%v", info.HasVideo, info.HasAudio)
	}
	if math.Abs(info.Duration-3) > 0.3 {
		t.Fatalf("mp3 is %.3fs, expected ~3s", info.Duration)
	}

	// A silent video must fail with a clear message rather than an empty mp3.
	if err := f.ExtractMP3(ctx, clip, filepath.Join(dir, "silent.mp3")); err == nil {
		t.Fatal("expected an error for a video without an audio track")
	}
}
