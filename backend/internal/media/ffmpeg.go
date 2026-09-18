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

// StretchMode selects how frames are retimed when a clip is fitted onto a
// soundtrack.
type StretchMode string

const (
	// StretchInterpolate synthesises the frames a slow-down needs from the motion
	// between the existing ones. Motion stays fluid at the price of a lot of CPU
	// (tens of seconds per clip) and of occasional warping where the estimator
	// guesses wrong.
	StretchInterpolate StretchMode = "interpolate"
	// StretchDuplicate simply holds each frame longer. Nearly free, but the
	// repeated frames are plainly visible as a stutter.
	StretchDuplicate StretchMode = "duplicate"
)

// ParseStretchMode validates a configured mode name, falling back to the
// interpolating one and reporting why.
func ParseStretchMode(s string) (StretchMode, error) {
	switch StretchMode(strings.ToLower(strings.TrimSpace(s))) {
	case StretchInterpolate, "":
		return StretchInterpolate, nil
	case StretchDuplicate:
		return StretchDuplicate, nil
	default:
		return StretchInterpolate, fmt.Errorf("unknown stretch mode %q", s)
	}
}

// Options configures the ffmpeg wrapper.
type Options struct {
	// Concurrency caps how many ffmpeg processes run at once. Encoding is CPU
	// bound, so this is deliberately lower than the worker pool size.
	Concurrency int
	// TempDir holds the scratch files. Empty means the OS default.
	TempDir string
	// StretchMode picks the retiming algorithm. The zero value interpolates.
	StretchMode StretchMode
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
	if o.StretchMode == "" {
		o.StretchMode = StretchInterpolate
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
	Width    int
	Height   int
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
		Width     int    `json:"width"`
		Height    int    `json:"height"`
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
			if info.Width == 0 && s.Width > 0 {
				info.Width, info.Height = s.Width, s.Height
			}
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

	// setpts stretches the presentation stamps; the retimed stream then has to be
	// filled up to OutputFPS, and how that is done is what the eye notices.
	var retime string
	if f.opts.StretchMode == StretchDuplicate {
		retime = fmt.Sprintf("[0:v]setpts=PTS*%.6f,fps=%d,format=yuv420p[v]", factor, f.opts.OutputFPS)
	} else {
		retime = fmt.Sprintf("[0:v]setpts=PTS*%.6f,minterpolate=fps=%d:%s,format=yuv420p[v]",
			factor, f.opts.OutputFPS, interpolateParams)
	}

	// A slow preset and a low crf are affordable here: the encode is already the
	// cheap half of an interpolated render, and re-encoding is where the softness
	// of the previous settings came from.
	return []string{
		"-nostdin", "-y",
		"-i", videoPath,
		"-i", audioPath,
		"-filter_complex", retime,
		"-map", "[v]",
		"-map", "1:a",
		"-c:v", "libx264", "-preset", "slow", "-crf", "18",
		"-c:a", "aac", "-b:a", "192k", "-ar", "44100",
		// Cut to the soundtrack so the result matches it exactly.
		"-t", strconv.FormatFloat(audioDur, 'f', 3, 64),
		"-movflags", "+faststart",
		outPath,
	}, nil
}

// interpolateParams tunes minterpolate for the least visible damage:
//   - mci builds each new frame along the estimated motion instead of blending
//     two frames together, which is what avoids ghosting;
//   - aobmc plus vsbmc soften the block edges plain motion compensation leaves;
//   - bidir estimates the motion from both surrounding frames;
//   - scd=none disables scene-change detection, whose fallback is exactly the
//     frame duplication we are trying to get away from — inside a single
//     continuous shot it only ever fires by mistake, as an abrupt stutter.
//
// The remaining knobs are left at their defaults on purpose: measured against a
// 48fps ground truth (halved to 24fps and rebuilt), a wider motion search
// (me=umh, search_param=64) and finer blocks (mb_size=8) both scored *worse*
// than the defaults while costing several times the CPU.
const interpolateParams = "mi_mode=mci:mc_mode=aobmc:me_mode=bidir:vsbmc=1:scd=none"

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
