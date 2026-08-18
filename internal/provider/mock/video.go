// video.go 实现 mock 供应商的视频能力：本地渲染动画帧 → exec ffmpeg 合成 H.264 MP4。
// text2video: 动态渐变 + 位移色块；img2video: 参考图 Ken Burns 推近，直观演示"画布连线语义"。
package mock

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tommax-bai/tommax-model-adapter-svc/internal/core"
)

func init() { _ = os.Setenv("PATH", os.Getenv("PATH")+":/opt/homebrew/bin:/usr/local/bin") }

const videoFPS = 16

func (p *Provider) runVideo(job *core.Job, update core.UpdateFn) {
	req := job.Request
	w := paramInt(req.Params, "width", 640, 128, 1280)
	h := paramInt(req.Params, "height", 360, 128, 720)
	dur := paramInt(req.Params, "durationSec", 3, 1, 6)
	// H.264 要求偶数尺寸。
	w, h = w-w%2, h-h%2
	frames := dur * videoFPS

	fail := func(status core.JobStatus, msg string) {
		update(job.ID, core.Result{Status: status, ErrorMsg: msg})
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		fail(core.StatusFailedPermanent, "ffmpeg 不可用："+err.Error())
		return
	}

	// img2video：拉取参考图作为画面基底。
	var ref image.Image
	if req.Capability == core.CapVideoImg2Vid {
		if len(req.RefURLs) == 0 {
			fail(core.StatusFailedPermanent, "img2video 需要参考图")
			return
		}
		ref, err = fetchImage(req.RefURLs[0])
		if err != nil {
			fail(core.StatusFailedRetryable, "拉取参考图失败："+err.Error())
			return
		}
	}

	dir, err := os.MkdirTemp("", "mockvideo-*")
	if err != nil {
		fail(core.StatusFailedRetryable, err.Error())
		return
	}
	defer os.RemoveAll(dir)

	for i := 0; i < frames; i++ {
		var frame image.Image
		if ref != nil {
			frame = kenBurns(ref, w, h, float64(i)/float64(frames-1))
		} else {
			frame = animatedAbstract(req.Prompt, w, h, i)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, frame); err != nil {
			fail(core.StatusFailedRetryable, err.Error())
			return
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f_%04d.png", i)), buf.Bytes(), 0o644); err != nil {
			fail(core.StatusFailedRetryable, err.Error())
			return
		}
		if i%videoFPS == 0 {
			update(job.ID, core.Result{Status: core.StatusRunning, Progress: 10 + i*60/frames})
		}
	}

	// WebM/VP8：开放编解码器，任何 Chromium 变体都能解（H.264 在部分环境缺解码器）。
	out := filepath.Join(dir, "out.webm")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-y",
		"-framerate", fmt.Sprint(videoFPS),
		"-i", filepath.Join(dir, "f_%04d.png"),
		"-c:v", "libvpx", "-b:v", "1M", "-pix_fmt", "yuv420p",
		"-deadline", "realtime", "-cpu-used", "8", out)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		tail := outBytes
		if len(tail) > 400 {
			tail = tail[len(tail)-400:]
		}
		fail(core.StatusFailedRetryable, fmt.Sprintf("ffmpeg: %v: %s", err, tail))
		return
	}
	update(job.ID, core.Result{Status: core.StatusRunning, Progress: 90})

	data, err := os.ReadFile(out)
	if err != nil {
		fail(core.StatusFailedRetryable, err.Error())
		return
	}
	update(job.ID, core.Result{
		Status: core.StatusSucceeded, Progress: 100,
		Outputs: []core.Output{{Data: data, MimeType: "video/webm", Width: w, Height: h}},
	})
}

// animatedAbstract 在静态 render 基础上按帧推进相位。
func animatedAbstract(prompt string, w, h, frame int) image.Image {
	return render(fmt.Sprintf("%s@t%d", prompt, frame/4), frame%4, w, h)
}

// kenBurns 对参考图做 cover 裁切 + 缓慢推近（1.0→1.18）。t ∈ [0,1]。
func kenBurns(src image.Image, w, h int, t float64) image.Image {
	zoom := 1.0 + 0.18*t
	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	// cover 缩放比。
	scale := max64(float64(w)/sw, float64(h)/sh) * zoom
	// 视口在源图中的尺寸与左上角（居中 + 轻微下移营造运动感）。
	vw, vh := float64(w)/scale, float64(h)/scale
	ox := (sw - vw) / 2
	oy := (sh-vh)/2 + (sh-vh)/2*0.2*t

	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + int(oy+float64(y)/scale)
		for x := 0; x < w; x++ {
			sx := sb.Min.X + int(ox+float64(x)/scale)
			r, g, b, _ := src.At(clamp(sx, sb.Min.X, sb.Max.X-1), clamp(sy, sb.Min.Y, sb.Max.Y-1)).RGBA()
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255})
		}
	}
	return dst
}

func fetchImage(url string) (image.Image, error) {
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 64<<20))
	return img, err
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
