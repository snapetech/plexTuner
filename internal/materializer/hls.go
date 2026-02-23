package materializer

import (
	"context"
	"fmt"
	"os/exec"
)

// materializeHLS writes an HLS (m3u8) stream to destPath as MP4 using ffmpeg remux (no transcode).
// Requires ffmpeg in PATH.
func materializeHLS(ctx context.Context, streamURL, destPath string) error {
	args := []string{
		"-y",
		"-i", streamURL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		"-movflags", "+faststart",
		destPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	return nil
}
