// Package mock 是占位供应商：不依赖任何外部 API，本地渲染确定性抽象图，
// 用于纵向切片离线跑通「提交 → 轮询 → 产物落库」全链路，并作为 provider 插件的参考实现。
package mock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"time"

	"github.com/tommax-bai/tommax-model-adapter-svc/internal/core"
)

type Provider struct {
	// Latency 模拟供应商推理时长，让上游轮询逻辑被真实经过。
	Latency time.Duration
}

func New(latency time.Duration) *Provider { return &Provider{Latency: latency} }

func (p *Provider) Name() string { return "mock" }

func (p *Provider) Submit(_ context.Context, job *core.Job, update core.UpdateFn) error {
	req := job.Request
	n := paramInt(req.Params, "n", 1, 1, 4)
	w := paramInt(req.Params, "width", 1024, 64, 2048)
	h := paramInt(req.Params, "height", 1024, 64, 2048)

	// 异步执行：作业生命周期不绑提交请求的 ctx（提交即受理）。
	go func() {
		steps := 4
		for i := 1; i <= steps; i++ {
			time.Sleep(p.Latency / time.Duration(steps))
			update(job.ID, core.Result{Status: core.StatusRunning, Progress: i * 100 / (steps + 1)})
		}
		outputs := make([]core.Output, 0, n)
		for i := 0; i < n; i++ {
			img := render(req.Prompt, i, w, h)
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				update(job.ID, core.Result{Status: core.StatusFailedRetryable, ErrorMsg: fmt.Sprintf("encode: %v", err)})
				return
			}
			outputs = append(outputs, core.Output{Data: buf.Bytes(), MimeType: "image/png", Width: w, Height: h})
		}
		update(job.ID, core.Result{Status: core.StatusSucceeded, Progress: 100, Outputs: outputs})
	}()
	return nil
}

func (p *Provider) Cancel(context.Context, *core.Job) error { return nil }

// render 生成与 prompt 确定性相关的抽象图：双色渐变底 + 哈希驱动的色块矩阵。
func render(prompt string, seed, w, h int) image.Image {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", prompt, seed)))
	top := color.NRGBA{R: sum[0], G: sum[1], B: sum[2], A: 255}
	bottom := color.NRGBA{R: sum[3], G: sum[4], B: sum[5], A: 255}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		c := color.NRGBA{
			R: lerp(top.R, bottom.R, t),
			G: lerp(top.G, bottom.G, t),
			B: lerp(top.B, bottom.B, t),
			A: 255,
		}
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	// 8x8 色块矩阵，位置与颜色由哈希后续字节决定，让不同 prompt 肉眼可分。
	cell := w / 8
	for i := 6; i < 30; i += 3 {
		cx := int(sum[i]) % 8
		cy := int(sum[i+1]) % 8
		blk := color.NRGBA{R: sum[i+2], G: sum[i], B: sum[i+1], A: 200}
		for y := cy * cell; y < (cy+1)*cell && y < h; y++ {
			for x := cx * cell; x < (cx+1)*cell && x < w; x++ {
				img.SetNRGBA(x, y, blend(img.NRGBAAt(x, y), blk))
			}
		}
	}
	return img
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

func blend(dst, src color.NRGBA) color.NRGBA {
	a := float64(src.A) / 255
	return color.NRGBA{
		R: uint8(float64(dst.R)*(1-a) + float64(src.R)*a),
		G: uint8(float64(dst.G)*(1-a) + float64(src.G)*a),
		B: uint8(float64(dst.B)*(1-a) + float64(src.B)*a),
		A: 255,
	}
}

func paramInt(params map[string]string, key string, def, min, max int) int {
	v, ok := params[key]
	if !ok {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil || i < min || i > max {
		return def
	}
	return i
}
