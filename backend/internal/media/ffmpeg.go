// Package media wraps the ffmpeg/ffprobe binaries used to inspect uploads and to
// fit a generated clip onto a soundtrack.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Options configures the ffmpeg wrapper.
type Options struct {
	// Concurrency caps how many ffmpeg processes run at once. Encoding is CPU
	// bound, so this is deliberately lower than the worker pool size.
	Concurrency int
	// TempDir holds the scratch files. Empty means the OS default.
	TempDir string
	// SmoothStretch trades CPU for quality: instead of holding frames longer,
	// missing frames are interpolated. Slow-motion looks fluid but a single clip
	// can take tens of seconds to encode.
	SmoothStretch bool
	// MaxStretchFactor refuses absurd slow-downs (a 4s clip over a 60s track
	// would be a slideshow, not a video).
	MaxStretchFactor float64
	// OutputFPS is the frame rate of the rendered video.
	OutputFPS int
	// Timeout bounds a single ffmpeg invocation.
	Timeout time.Duration
}

// FFmpeg runs ffmpeg/ffprobe commands.
type FFmpeg struct {
	opts Options
	// slots is a counting semaphore limiting concurrent encodes.
	slots chan struct{}
}

func New(o Options) *FFmpeg {
	if o.Concurrency < 1 {
		o.Concurrency = 1
	}
	if o.MaxStretchFactor <= 0 {
		o.MaxStretchFactor = 6
	}
	if o.OutputFPS < 1 {
		o.OutputFPS = 30
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Minute
	}
	return &FFmpeg{opts: o, slots: make(chan struct{}, o.Concurrency)}
}

// CheckTools verifies that both binaries are callable and logs their versions.
func (f *FFmpeg) CheckTools(ctx context.Context) error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := exec.CommandContext(ctx, bin, "-version").Output()
		cancel()
		if err != nil {
			return fmt.Errorf("%s is not available: %w", bin, err)
		}
		line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		log.Printf("media: %s", line)
	}
	return nil
}

// TempDir returns the scratch directory used for uploads and encodes.
func (f *FFmpeg) TempDir() string { return f.opts.TempDir }

// Info describes a media file as reported by ffprobe.
type Info struct {
	Duration float64
	HasVideo bool
	HasAudio bool
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Duration  string `json:"duration"`
	} `json:"streams"`
}

// Probe reads duration and stream layout from a file.
func (f *FFmpeg) Probe(ctx context.Context, path string) (*Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w: %s", filepath.Base(path), err, tail(stderr.String()))
	}

	var out probeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("ffprobe: parse output: %w", err)
	}

	info := &Info{}
	for _, s := range out.Streams {
		switch s.CodecType {
		case "video":
			info.HasVideo = true
		case "audio":
			info.HasAudio = true
		}
		// Container duration can be missing for some inputs; fall back to the
		// longest stream duration.
		if d, err := strconv.ParseFloat(s.Duration, 64); err == nil && d > info.Duration {
			info.Duration = d
		}
	}
	if d, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil && d > 0 {
		info.Duration = d
	}
	if info.Duration <= 0 {
		return nil, fmt.Errorf("ffprobe %s: could not determine duration", filepath.Base(path))
	}
	return info, nil
}

// StretchAndMux renders videoPath so that it lasts exactly as long as audioPath
// and carries that audio. A longer track slows the footage down, a shorter one
// speeds it up; either way the result matches the music exactly.
func (f *FFmpeg) StretchAndMux(ctx context.Context, videoPath, audioPath, outPath string) error {
	video, err := f.Probe(ctx, videoPath)
	if err != nil {
		return err
	}
	if !video.HasVideo {
		return fmt.Errorf("source file has no video stream")
	}
	audio, err := f.Probe(ctx, audioPath)
	if err != nil {
		return err
	}
	if !audio.HasAudio {
		return fmt.Errorf("soundtrack has no audio stream")
	}

	args, err := f.stretchArgs(videoPath, audioPath, outPath, video.Duration, audio.Duration)
	if err != nil {
		return err
	}
	return f.run(ctx, args)
}

// stretchArgs builds the ffmpeg invocation that retimes the video to audioDur
// and attaches the soundtrack.
func (f *FFmpeg) stretchArgs(videoPath, audioPath, outPath string, videoDur, audioDur float64) ([]string, error) {
	if videoDur <= 0 || audioDur <= 0 {
		return nil, fmt.Errorf("invalid durations: video %.3fs, audio %.3fs", videoDur, audioDur)
	}
	factor := audioDur / videoDur
	if factor > f.opts.MaxStretchFactor {
		return nil, fmt.Errorf(
			"soundtrack is %.1fs but the clip is only %.1fs — stretching it %.1fx would look like a slideshow (limit %.1fx)",
			audioDur, videoDur, factor, f.opts.MaxStretchFactor)
	}

	// setpts retimes the presentation stamps; fps then fills the timeline by
	// holding frames. minterpolate instead synthesises the missing ones.
	retime := fmt.Sprintf("[0:v]setpts=PTS*%.6f,fps=%d,format=yuv420p[v]", factor, f.opts.OutputFPS)
	if f.opts.SmoothStretch {
		retime = fmt.Sprintf(
			"[0:v]setpts=PTS*%.6f,minterpolate=fps=%d:mi_mode=mci:mc_mode=aobmc:vsbmc=1,format=yuv420p[v]",
			factor, f.opts.OutputFPS)
	}

	return []string{
		"-nostdin", "-y",
		"-i", videoPath,
		"-i", audioPath,
		"-filter_complex", retime,
		"-map", "[v]",
		"-map", "1:a",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100",
		// Cut to the soundtrack so the result matches it exactly.
		"-t", strconv.FormatFloat(audioDur, 'f', 3, 64),
		"-movflags", "+faststart",
		outPath,
	}, nil
}

// ExtractMP3 writes the audio track of a media file as an mp3.
func (f *FFmpeg) ExtractMP3(ctx context.Context, inPath, outPath string) error {
	info, err := f.Probe(ctx, inPath)
	if err != nil {
		return err
	}
	if !info.HasAudio {
		return fmt.Errorf("file has no audio track")
	}
	return f.run(ctx, []string{
		"-nostdin", "-y",
		"-i", inPath,
		"-vn",
		"-c:a", "libmp3lame", "-q:a", "2",
		outPath,
	})
}

// run executes ffmpeg while holding a concurrency slot.
func (f *FFmpeg) run(ctx context.Context, args []string) error {
	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}

	ctx, cancel := context.WithTimeout(ctx, f.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	started := time.Now()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, tail(stderr.String()))
	}
	log.Printf("media: ffmpeg finished in %s", time.Since(started).Round(time.Millisecond))
	return nil
}

// NewTempDir creates a scratch directory for one operation.
func (f *FFmpeg) NewTempDir(prefix string) (string, error) {
	return os.MkdirTemp(f.opts.TempDir, prefix)
}

// tail keeps the end of ffmpeg's stderr, where the actual reason lives.
func tail(s string) string {
	s = strings.TrimSpace(s)
	const limit = 600
	if len(s) > limit {
		return "…" + s[len(s)-limit:]
	}
	return s
}
