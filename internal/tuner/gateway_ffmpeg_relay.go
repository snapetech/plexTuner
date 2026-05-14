package tuner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type firstWriteLogger struct {
	w           io.Writer
	once        sync.Once
	channelName string
	channelID   string
	reqID       string
	modeLabel   string
	start       time.Time
}

func (f *firstWriteLogger) Write(p []byte) (int, error) {
	f.once.Do(func() {
		if f.modeLabel == "" {
			f.modeLabel = "ffmpeg-remux"
		}
		if f.reqID != "" {
			log.Printf("gateway: req=%s channel=%q id=%s %s first-bytes=%d startup=%s",
				f.reqID, f.channelName, f.channelID, f.modeLabel, len(p), time.Since(f.start).Round(time.Millisecond))
			return
		}
		log.Printf("gateway: channel=%q id=%s %s first-bytes=%d startup=%s",
			f.channelName, f.channelID, f.modeLabel, len(p), time.Since(f.start).Round(time.Millisecond))
	})
	return f.w.Write(p)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}

type hlsRelayFFmpegOutputWriter struct {
	w             http.ResponseWriter
	bodyOut       io.Writer
	flushBody     func()
	flusher       http.Flusher
	started       bool
	startedSignal *atomic.Bool
}

func (w *hlsRelayFFmpegOutputWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.w.Header().Set("Content-Type", "video/mp2t")
		w.w.Header().Del("Content-Length")
		w.w.WriteHeader(http.StatusOK)
		w.started = true
		if w.startedSignal != nil {
			w.startedSignal.Store(true)
		}
	}
	n, err := w.bodyOut.Write(p)
	if n > 0 {
		if w.flushBody != nil {
			w.flushBody()
		}
		if w.flusher != nil {
			w.flusher.Flush()
		}
		if w.startedSignal != nil {
			w.startedSignal.Store(true)
		}
	}
	return n, err
}

type hlsRelayFFmpegStdinNormalizer struct {
	stdin         io.WriteCloser
	done          chan error
	closeOnce     sync.Once
	responseStart atomic.Bool
}

func (n *hlsRelayFFmpegStdinNormalizer) Write(p []byte) (int, error) {
	if n == nil || n.stdin == nil {
		return 0, io.ErrClosedPipe
	}
	return n.stdin.Write(p)
}

func (n *hlsRelayFFmpegStdinNormalizer) ResponseStarted() bool {
	if n == nil {
		return false
	}
	return n.responseStart.Load()
}

func (n *hlsRelayFFmpegStdinNormalizer) CloseInput() error {
	if n == nil || n.stdin == nil {
		return nil
	}
	var err error
	n.closeOnce.Do(func() {
		err = n.stdin.Close()
	})
	return err
}

func (n *hlsRelayFFmpegStdinNormalizer) CloseAndWait() error {
	if n == nil {
		return nil
	}
	_ = n.CloseInput()
	if n.done == nil {
		return nil
	}
	return <-n.done
}

func (g *Gateway) startHLSRelayFFmpegStdinNormalizer(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	channelName string,
	channelID string,
	start time.Time,
	transcode bool,
	profile string,
	bodyOut io.Writer,
	flushBody func(),
	bufferBytes int,
	responseStarted bool,
	sharedSession *sharedRelaySession,
) (*hlsRelayFFmpegStdinNormalizer, error) {
	reqField := gatewayReqIDField(r.Context())
	modeLabel := "hls-relay-ffmpeg-stdin-remux"
	if transcode {
		modeLabel = "hls-relay-ffmpeg-stdin-transcode"
	}
	stdinAnalyzeDurationUs := getenvInt("IPTV_TUNERR_FFMPEG_STDIN_ANALYZEDURATION_US", 3000000)
	stdinProbeSize := getenvInt("IPTV_TUNERR_FFMPEG_STDIN_PROBESIZE", 3000000)
	stdinUseNoBuffer := getenvBool("IPTV_TUNERR_FFMPEG_STDIN_NOBUFFER", false)
	stdinLogLevel := strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_STDIN_LOGLEVEL"))
	if stdinLogLevel == "" {
		stdinLogLevel = strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_HLS_LOGLEVEL"))
	}
	if stdinLogLevel == "" {
		stdinLogLevel = "error"
	}
	fflags := "+discardcorrupt+genpts"
	if stdinUseNoBuffer {
		fflags += "+nobuffer"
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", stdinLogLevel,
		"-fflags", fflags,
		"-analyzeduration", strconv.Itoa(stdinAnalyzeDurationUs),
		"-probesize", strconv.Itoa(stdinProbeSize),
		"-f", "mpegts",
		"-i", "pipe:0",
	}
	args = append(args, buildFFmpegMPEGTSCodecArgs(transcode, profile)...)

	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	norm := &hlsRelayFFmpegStdinNormalizer{
		stdin: stdin,
		done:  make(chan error, 1),
	}
	if responseStarted {
		norm.responseStart.Store(true)
	}
	flusher, _ := w.(http.Flusher)
	out := &hlsRelayFFmpegOutputWriter{
		w:             w,
		bodyOut:       bodyOut,
		flushBody:     flushBody,
		flusher:       flusher,
		started:       responseStarted,
		startedSignal: &norm.responseStart,
	}
	dst := io.Writer(out)
	if sharedSession != nil {
		dst = &sharedRelayFanoutWriter{base: dst, session: sharedSession}
	}
	if flusher != nil {
		dst = &firstWriteLogger{
			w:           dst,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	} else {
		dst = &firstWriteLogger{
			w:           dst,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	}

	log.Printf("gateway:%s channel=%q id=%s %s start buffer=%d profile=%s analyzeduration_us=%d probesize=%d nobuffer=%t loglevel=%s",
		reqField, channelName, channelID, modeLabel, bufferBytes, profile, stdinAnalyzeDurationUs, stdinProbeSize, stdinUseNoBuffer, stdinLogLevel)
	go func() {
		nBytes, copyErr := io.Copy(dst, stdout)
		if flushBody != nil {
			flushBody()
		}
		waitErr := cmd.Wait()
		if r.Context().Err() != nil {
			log.Printf("gateway:%s channel=%q id=%s %s client-done bytes=%d dur=%s",
				reqField, channelName, channelID, modeLabel, nBytes, time.Since(start).Round(time.Millisecond))
			norm.done <- nil
			return
		}
		if copyErr != nil {
			norm.done <- ffmpegRelayErr("hls-relay-stdin-copy", copyErr, stderr.String())
			return
		}
		if waitErr != nil {
			norm.done <- ffmpegRelayErr("hls-relay-stdin-wait", waitErr, stderr.String())
			return
		}
		log.Printf("gateway:%s channel=%q id=%s %s bytes=%d dur=%s",
			reqField, channelName, channelID, modeLabel, nBytes, time.Since(start).Round(time.Millisecond))
		norm.done <- nil
	}()
	return norm, nil
}

func writeBootstrapTS(ctx context.Context, ffmpegPath string, dst io.Writer, channelName, channelID string, seconds float64, profile string) error {
	if seconds <= 0 {
		return nil
	}
	if seconds > 5 {
		seconds = 5
	}
	mpegtsFlags := mpegTSFlagsWithOptionalInitialDiscontinuity()
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1280x720:r=30000/1001",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", strconv.FormatFloat(seconds, 'f', 2, 64),
		"-shortest",
		"-map", "0:v:0",
	}
	if normalizeProfileName(profile) != profileVideoOnly {
		args = append(args, "-map", "1:a:0")
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1:nal-hrd=cbr",
		"-b:v", "900k",
		"-maxrate", "900k",
		"-bufsize", "900k",
	)
	args = append(args, bootstrapAudioArgsForProfile(profile)...)
	args = append(args,
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", mpegtsFlags,
		"-f", "mpegts",
		"pipe:1",
	)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	n, copyErr := io.Copy(dst, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return errors.New(msg)
	}
	log.Printf("gateway:%s channel=%q id=%s bootstrap-ts bytes=%d dur=%.2fs profile=%s",
		gatewayReqIDField(ctx), channelName, channelID, n, seconds, normalizeProfileName(profile))
	return nil
}
