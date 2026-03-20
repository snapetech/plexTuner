package tuner

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type tsDiscontinuitySpliceWriter struct {
	dst        io.Writer
	reqField   string
	channel    string
	channelID  string
	seenPIDs   map[uint16]struct{}
	buf        []byte
	emitted    int64
	shimPkts   int
	rawPackets int
	active     bool
	maxPIDs    int
}

func newTSDiscontinuitySpliceWriter(ctx context.Context, dst io.Writer, channelName, channelID string) *tsDiscontinuitySpliceWriter {
	return &tsDiscontinuitySpliceWriter{
		dst:       dst,
		reqField:  gatewayReqIDField(ctx),
		channel:   channelName,
		channelID: channelID,
		seenPIDs:  make(map[uint16]struct{}, 8),
		active:    true,
		maxPIDs:   16,
	}
}

func makeTSDiscontinuityPacket(pid uint16, cc byte) [188]byte {
	var pkt [188]byte
	pkt[0] = 0x47
	pkt[1] = byte((pid >> 8) & 0x1F)
	pkt[2] = byte(pid & 0xFF)
	pkt[3] = 0x20 | (cc & 0x0F)
	pkt[4] = 183
	pkt[5] = 0x80
	for i := 6; i < len(pkt); i++ {
		pkt[i] = 0xFF
	}
	return pkt
}

func (w *tsDiscontinuitySpliceWriter) writePacket(pkt []byte) error {
	if len(pkt) != 188 {
		_, err := w.dst.Write(pkt)
		if err == nil {
			w.emitted += int64(len(pkt))
		}
		return err
	}
	if w.active {
		if pkt[0] != 0x47 {
			w.active = false
			log.Printf("gateway:%s channel=%q id=%s hls-relay splice-discontinuity disable reason=lost-sync head=%x",
				w.reqField, w.channel, w.channelID, pkt[:min(len(pkt), 8)])
		} else {
			pid := uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
			if pid != 0x1FFF {
				if _, ok := w.seenPIDs[pid]; !ok && len(w.seenPIDs) < w.maxPIDs {
					shim := makeTSDiscontinuityPacket(pid, pkt[3]&0x0F)
					if _, err := w.dst.Write(shim[:]); err != nil {
						return err
					}
					w.emitted += int64(len(shim))
					w.shimPkts++
					w.seenPIDs[pid] = struct{}{}
				}
			}
			if len(w.seenPIDs) >= w.maxPIDs {
				w.active = false
			}
		}
	}
	_, err := w.dst.Write(pkt)
	if err == nil {
		w.emitted += int64(len(pkt))
		w.rawPackets++
	}
	return err
}

func (w *tsDiscontinuitySpliceWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil || len(p) == 0 {
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for len(w.buf) >= 188 {
		if err := w.writePacket(w.buf[:188]); err != nil {
			return 0, err
		}
		w.buf = w.buf[188:]
	}
	return len(p), nil
}

func (w *tsDiscontinuitySpliceWriter) FlushRemainder() error {
	if w == nil {
		return nil
	}
	if len(w.buf) > 0 {
		if _, err := w.dst.Write(w.buf); err != nil {
			return err
		}
		w.emitted += int64(len(w.buf))
		w.buf = nil
	}
	log.Printf("gateway:%s channel=%q id=%s hls-relay splice-discontinuity shims=%d unique_pids=%d raw_packets=%d",
		w.reqField, w.channel, w.channelID, w.shimPkts, len(w.seenPIDs), w.rawPackets)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type startSignalState struct {
	TSLikePackets int
	HasIDR        bool
	HasAAC        bool
	AlignedOffset int
}

func containsH264IDRAnnexB(buf []byte) bool {
	if len(buf) < 4 {
		return false
	}
	for i := 0; i < len(buf)-3; i++ {
		if i+4 <= len(buf) && buf[i] == 0x00 && buf[i+1] == 0x00 && buf[i+2] == 0x01 {
			if (buf[i+3] & 0x1f) == 5 {
				return true
			}
			continue
		}
		if i+5 <= len(buf) && buf[i] == 0x00 && buf[i+1] == 0x00 && buf[i+2] == 0x00 && buf[i+3] == 0x01 {
			if (buf[i+4] & 0x1f) == 5 {
				return true
			}
		}
	}
	return false
}

const tsPacketSize = 188

// trimTSHeadToMaxBytes drops MPEG-TS 188-byte packets from the front until len(buf) <= maxBytes,
// resyncing on 0x47. Used by the WebSafe startup prefetch loop (HR-001) so we can keep reading
// for an IDR while bounding memory. If no sync byte exists, retains the tail up to maxBytes.
func trimTSHeadToMaxBytes(buf []byte, maxBytes int) []byte {
	if maxBytes <= 0 || len(buf) <= maxBytes {
		return buf
	}
	for len(buf) > maxBytes {
		if len(buf) < tsPacketSize {
			break
		}
		if buf[0] == 0x47 {
			buf = buf[tsPacketSize:]
			continue
		}
		idx := bytes.IndexByte(buf[1:], 0x47)
		if idx < 0 {
			if len(buf) > maxBytes {
				buf = buf[len(buf)-maxBytes:]
			}
			break
		}
		buf = buf[idx+1:]
	}
	return buf
}

func looksLikeGoodTSStart(buf []byte) startSignalState {
	pkt := tsPacketSize
	st := startSignalState{}
	st.AlignedOffset = -1
	var idrCarry []byte
	for off := 0; off+pkt <= len(buf); {
		if buf[off] != 0x47 {
			n := bytes.IndexByte(buf[off+1:], 0x47)
			if n < 0 {
				break
			}
			off += n + 1
			continue
		}
		st.TSLikePackets++
		if st.AlignedOffset < 0 {
			ok := 0
			for k := off; k < len(buf) && ok < 4; k += pkt {
				if k >= len(buf) || buf[k] != 0x47 {
					break
				}
				ok++
			}
			if ok >= 3 {
				st.AlignedOffset = off
			}
		}
		p := buf[off : off+pkt]
		afc := (p[3] >> 4) & 0x3
		i := 4
		if afc == 0 || afc == 2 {
			off += pkt
			continue
		}
		if afc == 3 {
			if i >= len(p) {
				off += pkt
				continue
			}
			alen := int(p[i])
			i++
			i += alen
		}
		if i >= len(p) {
			off += pkt
			continue
		}
		payload := p[i:]
		if !st.HasIDR {
			if containsH264IDRAnnexB(payload) {
				st.HasIDR = true
			} else if len(idrCarry) > 0 {
				joined := make([]byte, 0, len(idrCarry)+len(payload))
				joined = append(joined, idrCarry...)
				joined = append(joined, payload...)
				if containsH264IDRAnnexB(joined) {
					st.HasIDR = true
				}
			}
		}
		if !st.HasAAC {
			for j := 0; j+1 < len(payload); j++ {
				if payload[j] == 0xFF && (payload[j+1]&0xF0) == 0xF0 {
					st.HasAAC = true
					break
				}
			}
		}
		if len(payload) > 0 {
			if len(payload) >= 4 {
				idrCarry = append(idrCarry[:0], payload[len(payload)-4:]...)
			} else {
				keep := len(idrCarry) + len(payload)
				if keep > 4 {
					drop := keep - 4
					if drop < len(idrCarry) {
						idrCarry = idrCarry[drop:]
					} else {
						idrCarry = idrCarry[:0]
					}
				}
				idrCarry = append(idrCarry, payload...)
				if len(idrCarry) > 4 {
					idrCarry = idrCarry[len(idrCarry)-4:]
				}
			}
		}
		if st.HasIDR && st.HasAAC && st.TSLikePackets >= 8 {
			return st
		}
		off += pkt
	}
	return st
}

const (
	adaptiveBufferMin       = 64 << 10
	adaptiveBufferMax       = 2 << 20
	adaptiveBufferInitial   = 1 << 20
	adaptiveSlowFlushMs     = 100
	adaptiveFastFlushMs     = 20
	adaptiveFastCountShrink = 3
)

type adaptiveWriter struct {
	w            io.Writer
	buf          bytes.Buffer
	targetSize   int
	minSize      int
	maxSize      int
	slowThresh   time.Duration
	fastThresh   time.Duration
	fastCount    int
	fastCountMax int
}

func newAdaptiveWriter(w io.Writer) *adaptiveWriter {
	return &adaptiveWriter{
		w:            w,
		targetSize:   adaptiveBufferInitial,
		minSize:      adaptiveBufferMin,
		maxSize:      adaptiveBufferMax,
		slowThresh:   adaptiveSlowFlushMs * time.Millisecond,
		fastThresh:   adaptiveFastFlushMs * time.Millisecond,
		fastCountMax: adaptiveFastCountShrink,
	}
}

func (a *adaptiveWriter) Write(p []byte) (int, error) {
	n, err := a.buf.Write(p)
	if err != nil {
		return n, err
	}
	for a.buf.Len() >= a.targetSize {
		if err := a.flushToClient(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (a *adaptiveWriter) flushToClient() error {
	if a.buf.Len() == 0 {
		return nil
	}
	start := time.Now()
	for a.buf.Len() > 0 {
		n, err := a.w.Write(a.buf.Bytes())
		if err != nil {
			return err
		}
		if n <= 0 {
			break
		}
		remaining := a.buf.Bytes()[n:]
		a.buf.Reset()
		a.buf.Write(remaining)
	}
	d := time.Since(start)
	if d >= a.slowThresh {
		if a.targetSize < a.maxSize {
			a.targetSize *= 2
			if a.targetSize > a.maxSize {
				a.targetSize = a.maxSize
			}
		}
		a.fastCount = 0
	} else if d <= a.fastThresh {
		a.fastCount++
		if a.fastCount >= a.fastCountMax {
			a.fastCount = 0
			if a.targetSize > a.minSize {
				a.targetSize /= 2
				if a.targetSize < a.minSize {
					a.targetSize = a.minSize
				}
			}
		}
	} else {
		a.fastCount = 0
	}
	return nil
}

func (a *adaptiveWriter) Flush() error { return a.flushToClient() }

func streamWriter(w http.ResponseWriter, bufferBytes int) (io.Writer, func()) {
	if bufferBytes == 0 {
		return w, func() {}
	}
	if bufferBytes == -1 {
		aw := newAdaptiveWriter(w)
		return aw, func() { _ = aw.Flush() }
	}
	bw := bufio.NewWriterSize(w, bufferBytes)
	return bw, func() { _ = bw.Flush() }
}

func startNullTSKeepalive(
	ctx context.Context,
	dst io.Writer,
	flushBody func(),
	flusher http.Flusher,
	channelName, channelID, modeLabel string,
	start time.Time,
	interval time.Duration,
	packetsPerTick int,
) func(string) {
	if dst == nil || interval <= 0 || packetsPerTick <= 0 {
		return func(string) {}
	}
	if interval < 25*time.Millisecond {
		interval = 25 * time.Millisecond
	}
	if packetsPerTick > 64 {
		packetsPerTick = 64
	}
	stopCh := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		reqField := gatewayReqIDField(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		pkt := [188]byte{0x47, 0x1F, 0xFF, 0x10}
		for i := 4; i < len(pkt); i++ {
			pkt[i] = 0xFF
		}
		var sentBytes int64
		var ticks int
		reason := "done"
		for {
			select {
			case <-ctx.Done():
				reason = "client-done"
				log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case reason = <-stopCh:
				log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case <-ticker.C:
			}
			for i := 0; i < packetsPerTick; i++ {
				n, err := dst.Write(pkt[:])
				if n > 0 {
					sentBytes += int64(n)
				}
				if err != nil {
					reason = "write-error"
					log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s err=%v bytes=%d ticks=%d startup=%s",
						reqField, channelName, channelID, modeLabel, reason, err, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
					return
				}
			}
			ticks++
			if flushBody != nil {
				flushBody()
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}()
	var once sync.Once
	return func(reason string) {
		once.Do(func() {
			select {
			case stopCh <- reason:
			default:
			}
			<-done
		})
	}
}
