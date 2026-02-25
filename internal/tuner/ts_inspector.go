package tuner

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type tsInspectOptions struct {
	Enabled      bool
	MaxPackets   int
	ChannelMatch string
}

func tsInspectOptionsFromEnv() tsInspectOptions {
	maxPackets := getenvInt("PLEX_TUNER_TS_INSPECT_MAX_PACKETS", 12000)
	if maxPackets <= 0 {
		maxPackets = 12000
	}
	return tsInspectOptions{
		Enabled:      getenvBool("PLEX_TUNER_TS_INSPECT", false),
		MaxPackets:   maxPackets,
		ChannelMatch: strings.ToLower(strings.TrimSpace(os.Getenv("PLEX_TUNER_TS_INSPECT_CHANNEL"))),
	}
}

func (o tsInspectOptions) shouldInspect(channelName, channelID, guideNumber, tvgID string) bool {
	if !o.Enabled {
		return false
	}
	if o.ChannelMatch == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{channelName, channelID, guideNumber, tvgID}, " "))
	return strings.Contains(hay, o.ChannelMatch)
}

type tsInspectorWriter struct {
	dst   io.Writer
	inner *tsInspector
}

func (w *tsInspectorWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil {
		return 0, io.ErrClosedPipe
	}
	n, err := w.dst.Write(p)
	if n > 0 && w.inner != nil {
		w.inner.Observe(p[:n])
	}
	return n, err
}

func (w *tsInspectorWriter) Close() {
	if w == nil || w.inner == nil {
		return
	}
	w.inner.Close()
}

func maybeWrapTSInspectorWriter(
	dst io.Writer,
	reqID string,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	modeLabel string,
	start time.Time,
) io.Writer {
	opts := tsInspectOptionsFromEnv()
	if !opts.shouldInspect(channelName, channelID, guideNumber, tvgID) {
		return dst
	}
	ins := newTSInspector(reqID, channelName, channelID, guideNumber, tvgID, modeLabel, start, opts.MaxPackets)
	return &tsInspectorWriter{dst: dst, inner: ins}
}

type tsPIDStats struct {
	PID               uint16
	StreamType        byte
	StreamTypeKnown   bool
	Packets           int
	PayloadPackets    int
	PUSI              int
	CCSeen            bool
	LastCC            byte
	CCErrors          int
	CCDup             int
	DiscIndicatorPkts int
	PCRCount          int
	PCRFirst          uint64
	PCRLast           uint64
	PCRBackwards      int
	PCRMinDelta       uint64
	PCRMaxDelta       uint64
	PTSCount          int
	PTSFirst          uint64
	PTSLast           uint64
	PTSBackwards      int
	PTSMinDelta       uint64
	PTSMaxDelta       uint64
	DTSCount          int
	DTSFirst          uint64
	DTSLast           uint64
	DTSBackwards      int
	DTSMinDelta       uint64
	DTSMaxDelta       uint64
}

type tsInspector struct {
	reqID       string
	channelName string
	channelID   string
	guideNumber string
	tvgID       string
	modeLabel   string
	start       time.Time
	maxPackets  int

	mu sync.Mutex

	buf          []byte
	closed       bool
	loggedDone   bool
	packets      int
	syncLosses   int
	totalBytes   int64
	globalCCErrs int
	globalCCDup  int
	globalDisc   int

	patCount  int
	pmtCount  int
	pmtPID    uint16
	pmtPIDSet bool
	pcrPID    uint16
	pcrPIDSet bool

	pids map[uint16]*tsPIDStats
}

func newTSInspector(
	reqID string,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	modeLabel string,
	start time.Time,
	maxPackets int,
) *tsInspector {
	ins := &tsInspector{
		reqID:       reqID,
		channelName: channelName,
		channelID:   channelID,
		guideNumber: guideNumber,
		tvgID:       tvgID,
		modeLabel:   modeLabel,
		start:       start,
		maxPackets:  maxPackets,
		pids:        map[uint16]*tsPIDStats{},
	}
	log.Printf("gateway:%s channel=%q id=%s %s ts-inspect start max_packets=%d guide=%q tvg=%q",
		reqIDField(reqID), channelName, channelID, modeLabelLabel(modeLabel), maxPackets, guideNumber, tvgID)
	return ins
}

func reqIDField(reqID string) string {
	if reqID == "" {
		return ""
	}
	return " req=" + reqID
}

func modeLabelLabel(v string) string {
	if strings.TrimSpace(v) == "" {
		return "ffmpeg-remux"
	}
	return v
}

func (t *tsInspector) Observe(p []byte) {
	if t == nil || len(p) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.totalBytes += int64(len(p))
	if t.loggedDone {
		return
	}
	t.buf = append(t.buf, p...)
	for t.packets < t.maxPackets {
		if len(t.buf) < 188 {
			return
		}
		if t.buf[0] != 0x47 {
			n := bytes.IndexByte(t.buf[1:], 0x47)
			if n < 0 {
				// Keep a small tail so we can resync on the next write.
				if len(t.buf) > 187 {
					t.buf = append(t.buf[:0], t.buf[len(t.buf)-187:]...)
				}
				t.syncLosses++
				return
			}
			t.buf = t.buf[n+1:]
			t.syncLosses++
			continue
		}
		pkt := make([]byte, 188)
		copy(pkt, t.buf[:188])
		t.buf = t.buf[188:]
		t.observePacket(pkt)
		if t.packets >= t.maxPackets {
			t.logSummaryLocked("packet-limit")
			return
		}
	}
}

func (t *tsInspector) Close() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if !t.loggedDone {
		t.logSummaryLocked("close")
	}
}

func (t *tsInspector) pidStat(pid uint16) *tsPIDStats {
	s := t.pids[pid]
	if s != nil {
		return s
	}
	s = &tsPIDStats{PID: pid}
	t.pids[pid] = s
	return s
}

func (t *tsInspector) observePacket(pkt []byte) {
	if len(pkt) != 188 || pkt[0] != 0x47 {
		return
	}
	t.packets++
	pid := (uint16(pkt[1]&0x1F) << 8) | uint16(pkt[2])
	pusi := (pkt[1] & 0x40) != 0
	afc := (pkt[3] >> 4) & 0x03
	cc := pkt[3] & 0x0F
	hasPayload := afc == 1 || afc == 3
	hasAdapt := afc == 2 || afc == 3

	s := t.pidStat(pid)
	s.Packets++
	if pusi {
		s.PUSI++
	}

	discIndicator := false
	payloadOff := 4
	if hasAdapt {
		if payloadOff < len(pkt) {
			alen := int(pkt[payloadOff])
			payloadOff++
			if payloadOff+alen <= len(pkt) && alen > 0 {
				flags := pkt[payloadOff]
				discIndicator = (flags & 0x80) != 0
				if discIndicator {
					s.DiscIndicatorPkts++
					t.globalDisc++
				}
				if (flags&0x10) != 0 && alen >= 7 {
					if pcr, ok := parseTSPCR(pkt[payloadOff+1 : payloadOff+7]); ok {
						recordTick27MHz(&s.PCRCount, &s.PCRFirst, &s.PCRLast, &s.PCRBackwards, &s.PCRMinDelta, &s.PCRMaxDelta, pcr)
					}
				}
			}
			payloadOff += alen
		}
	}

	if hasPayload {
		s.PayloadPackets++
		if s.CCSeen {
			exp := (s.LastCC + 1) & 0x0F
			if cc != exp {
				if discIndicator {
					// Discontinuity signaled: reset continuity expectations.
				} else if cc == s.LastCC {
					s.CCDup++
					t.globalCCDup++
				} else {
					s.CCErrors++
					t.globalCCErrs++
				}
			}
		}
		s.CCSeen = true
		s.LastCC = cc
	}

	if !hasPayload || payloadOff >= len(pkt) {
		return
	}
	payload := pkt[payloadOff:]
	if pid == 0 && pusi {
		if t.parsePAT(payload) {
			t.patCount++
		}
		return
	}
	if t.pmtPIDSet && pid == t.pmtPID && pusi {
		if t.parsePMT(payload) {
			t.pmtCount++
		}
		return
	}
	if pusi {
		if pts, dts, hasPTS, hasDTS := parsePESPTSDTS(payload); hasPTS || hasDTS {
			if hasPTS {
				recordTick90k(&s.PTSCount, &s.PTSFirst, &s.PTSLast, &s.PTSBackwards, &s.PTSMinDelta, &s.PTSMaxDelta, pts)
			}
			if hasDTS {
				recordTick90k(&s.DTSCount, &s.DTSFirst, &s.DTSLast, &s.DTSBackwards, &s.DTSMinDelta, &s.DTSMaxDelta, dts)
			}
		}
	}
}

func (t *tsInspector) parsePAT(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	ptr := int(payload[0])
	if 1+ptr >= len(payload) {
		return false
	}
	sec := payload[1+ptr:]
	if len(sec) < 8 || sec[0] != 0x00 {
		return false
	}
	sectionLen := int(sec[1]&0x0F)<<8 | int(sec[2])
	if sectionLen < 9 || 3+sectionLen > len(sec) {
		return false
	}
	end := 3 + sectionLen
	for i := 8; i+4 <= end-4; i += 4 {
		progNum := uint16(sec[i])<<8 | uint16(sec[i+1])
		pid := (uint16(sec[i+2]&0x1F) << 8) | uint16(sec[i+3])
		if progNum != 0 {
			t.pmtPID = pid
			t.pmtPIDSet = true
			return true
		}
	}
	return false
}

func (t *tsInspector) parsePMT(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	ptr := int(payload[0])
	if 1+ptr >= len(payload) {
		return false
	}
	sec := payload[1+ptr:]
	if len(sec) < 12 || sec[0] != 0x02 {
		return false
	}
	sectionLen := int(sec[1]&0x0F)<<8 | int(sec[2])
	if sectionLen < 13 || 3+sectionLen > len(sec) {
		return false
	}
	end := 3 + sectionLen
	t.pcrPID = (uint16(sec[8]&0x1F) << 8) | uint16(sec[9])
	t.pcrPIDSet = true
	progInfoLen := int(sec[10]&0x0F)<<8 | int(sec[11])
	i := 12 + progInfoLen
	if i > end-4 {
		return true
	}
	for i+5 <= end-4 {
		stype := sec[i]
		pid := (uint16(sec[i+1]&0x1F) << 8) | uint16(sec[i+2])
		esInfoLen := int(sec[i+3]&0x0F)<<8 | int(sec[i+4])
		s := t.pidStat(pid)
		s.StreamType = stype
		s.StreamTypeKnown = true
		i += 5 + esInfoLen
	}
	return true
}

func (t *tsInspector) logSummaryLocked(reason string) {
	if t.loggedDone {
		return
	}
	t.loggedDone = true
	reqField := reqIDField(t.reqID)
	modeLabel := modeLabelLabel(t.modeLabel)
	log.Printf("gateway:%s channel=%q id=%s %s ts-inspect summary reason=%s packets=%d bytes=%d sync_losses=%d pat=%d pmt=%d pmt_pid=%s pcr_pid=%s pids=%d cc_err=%d cc_dup=%d disc=%d dur=%s",
		reqField, t.channelName, t.channelID, modeLabel, reason, t.packets, t.totalBytes, t.syncLosses,
		t.patCount, t.pmtCount, formatPIDMaybe(t.pmtPIDSet, t.pmtPID), formatPIDMaybe(t.pcrPIDSet, t.pcrPID),
		len(t.pids), t.globalCCErrs, t.globalCCDup, t.globalDisc, time.Since(t.start).Round(time.Millisecond))
	if len(t.pids) == 0 {
		return
	}
	type row struct {
		pid uint16
		s   *tsPIDStats
	}
	rows := make([]row, 0, len(t.pids))
	for pid, s := range t.pids {
		rows = append(rows, row{pid: pid, s: s})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].s.Packets == rows[j].s.Packets {
			return rows[i].pid < rows[j].pid
		}
		return rows[i].s.Packets > rows[j].s.Packets
	})
	limit := 12
	if len(rows) < limit {
		limit = len(rows)
	}
	for i := 0; i < limit; i++ {
		s := rows[i].s
		stream := "-"
		if s.StreamTypeKnown {
			stream = tsStreamTypeName(s.StreamType)
		}
		flags := []string{}
		if t.pcrPIDSet && s.PID == t.pcrPID {
			flags = append(flags, "PCR")
		}
		if t.pmtPIDSet && s.PID == t.pmtPID {
			flags = append(flags, "PMT")
		}
		if s.PID == 0 {
			flags = append(flags, "PAT")
		}
		flagText := "-"
		if len(flags) > 0 {
			flagText = strings.Join(flags, ",")
		}
		log.Printf("gateway:%s channel=%q id=%s %s ts-inspect pid=%s flags=%s stream=%s pkts=%d payload=%d pusi=%d cc_err=%d cc_dup=%d disc=%d pcr=%s pts=%s dts=%s",
			reqField, t.channelName, t.channelID, modeLabel, formatPIDHex(s.PID), flagText, stream,
			s.Packets, s.PayloadPackets, s.PUSI, s.CCErrors, s.CCDup, s.DiscIndicatorPkts,
			formatTick27Summary(s.PCRCount, s.PCRFirst, s.PCRLast, s.PCRBackwards, s.PCRMinDelta, s.PCRMaxDelta),
			formatTick90Summary(s.PTSCount, s.PTSFirst, s.PTSLast, s.PTSBackwards, s.PTSMinDelta, s.PTSMaxDelta),
			formatTick90Summary(s.DTSCount, s.DTSFirst, s.DTSLast, s.DTSBackwards, s.DTSMinDelta, s.DTSMaxDelta),
		)
	}
}

func parseTSPCR(b []byte) (uint64, bool) {
	if len(b) < 6 {
		return 0, false
	}
	base := (uint64(b[0]) << 25) |
		(uint64(b[1]) << 17) |
		(uint64(b[2]) << 9) |
		(uint64(b[3]) << 1) |
		(uint64(b[4]) >> 7)
	ext := (uint64(b[4]&0x01) << 8) | uint64(b[5])
	return base*300 + ext, true
}

func parsePESPTSDTS(payload []byte) (pts uint64, dts uint64, hasPTS bool, hasDTS bool) {
	if len(payload) < 14 {
		return 0, 0, false, false
	}
	if payload[0] != 0x00 || payload[1] != 0x00 || payload[2] != 0x01 {
		return 0, 0, false, false
	}
	flags2 := payload[7]
	hdrLen := int(payload[8])
	if 9+hdrLen > len(payload) {
		return 0, 0, false, false
	}
	ptsDtsFlags := (flags2 >> 6) & 0x03
	off := 9
	if ptsDtsFlags == 0x02 || ptsDtsFlags == 0x03 {
		if off+5 > len(payload) {
			return 0, 0, false, false
		}
		if v, ok := parseMPEGTimestamp33(payload[off : off+5]); ok {
			pts, hasPTS = v, true
		}
		off += 5
	}
	if ptsDtsFlags == 0x03 {
		if off+5 > len(payload) {
			return pts, 0, hasPTS, false
		}
		if v, ok := parseMPEGTimestamp33(payload[off : off+5]); ok {
			dts, hasDTS = v, true
		}
	}
	return pts, dts, hasPTS, hasDTS
}

func parseMPEGTimestamp33(b []byte) (uint64, bool) {
	if len(b) < 5 {
		return 0, false
	}
	// MPEG PES timestamp uses marker bits in bytes 0/2/4 low bit.
	if (b[0]&0x01) != 0x01 || (b[2]&0x01) != 0x01 || (b[4]&0x01) != 0x01 {
		return 0, false
	}
	v := (uint64((b[0]>>1)&0x07) << 30) |
		(uint64(b[1]) << 22) |
		(uint64((b[2]>>1)&0x7F) << 15) |
		(uint64(b[3]) << 7) |
		uint64((b[4]>>1)&0x7F)
	return v, true
}

func recordTick27MHz(count *int, first *uint64, last *uint64, backwards *int, minDelta *uint64, maxDelta *uint64, v uint64) {
	recordTickGeneric(count, first, last, backwards, minDelta, maxDelta, v)
}

func recordTick90k(count *int, first *uint64, last *uint64, backwards *int, minDelta *uint64, maxDelta *uint64, v uint64) {
	recordTickGeneric(count, first, last, backwards, minDelta, maxDelta, v)
}

func recordTickGeneric(count *int, first *uint64, last *uint64, backwards *int, minDelta *uint64, maxDelta *uint64, v uint64) {
	if *count == 0 {
		*first = v
		*last = v
		*count = 1
		return
	}
	if v < *last {
		*backwards = *backwards + 1
	} else {
		d := v - *last
		if *count == 1 || d < *minDelta {
			*minDelta = d
		}
		if d > *maxDelta {
			*maxDelta = d
		}
	}
	*last = v
	*count = *count + 1
}

func formatPIDMaybe(ok bool, pid uint16) string {
	if !ok {
		return "-"
	}
	return formatPIDHex(pid)
}

func formatPIDHex(pid uint16) string {
	return "0x" + strings.ToUpper(strconv.FormatUint(uint64(pid), 16))
}

func tsStreamTypeName(t byte) string {
	switch t {
	case 0x01:
		return "mpeg1video"
	case 0x02:
		return "mpeg2video"
	case 0x03:
		return "mpeg1audio"
	case 0x04:
		return "mpeg2audio"
	case 0x0f:
		return "aac"
	case 0x1b:
		return "h264"
	case 0x24:
		return "hevc"
	case 0x06:
		return "private"
	default:
		return fmt.Sprintf("0x%02x", t)
	}
}

func formatTick27Summary(count int, first uint64, last uint64, backwards int, minDelta uint64, maxDelta uint64) string {
	if count == 0 {
		return "-"
	}
	return fmt.Sprintf("n=%d first=%.3fms last=%.3fms span=%.3fms back=%d dmin=%.3fms dmax=%.3fms",
		count, float64(first)/27000.0, float64(last)/27000.0, float64(last-first)/27000.0, backwards,
		float64(minDelta)/27000.0, float64(maxDelta)/27000.0)
}

func formatTick90Summary(count int, first uint64, last uint64, backwards int, minDelta uint64, maxDelta uint64) string {
	if count == 0 {
		return "-"
	}
	return fmt.Sprintf("n=%d first=%.3fms last=%.3fms span=%.3fms back=%d dmin=%.3fms dmax=%.3fms",
		count, float64(first)/90.0, float64(last)/90.0, float64(last-first)/90.0, backwards,
		float64(minDelta)/90.0, float64(maxDelta)/90.0)
}
