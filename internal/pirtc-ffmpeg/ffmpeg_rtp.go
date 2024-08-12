package pirtc_ffmpeg

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
)

type FFmpegRTP struct {
    cmd *exec.Cmd
    ctx context.Context
    cancel context.CancelFunc
}

func NewFFmpegRTP(input, output string) *FFmpegRTP {
    ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "ffmpeg", "-f", "v4l2", "-i", input, "-vf", "format=yuv420p", "-c:v", "libx264", "-preset", "superfast", "-tune", "zerolatency", "-b:v", "500k", "-f", "rtp", output+"?pkt_size=100")

    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    return &FFmpegRTP{cmd: cmd, ctx: ctx, cancel: cancel}
}

func (f *FFmpegRTP) Start() error {
    err := f.cmd.Start()
    if err != nil {
        return fmt.Errorf("failed to start ffmpeg: %v", err)
    }
    log.Println("FFmpeg process started")
    return nil
}

func (f *FFmpegRTP) Stop() error {
	log.Println("Call ffmpeg stop")
    if f.cancel != nil {
        f.cancel()
        log.Println("Canceled FFmpeg context")
        if err := f.cmd.Process.Signal(os.Interrupt); err != nil {
            log.Printf("Failed to send SIGTERM to ffmpeg: %v", err)
            return fmt.Errorf("failed to send SIGTERM to ffmpeg process: %v", err)
        }
        log.Println("SIGTERM signal sent to ffmpeg process")
    }
    return nil
}
