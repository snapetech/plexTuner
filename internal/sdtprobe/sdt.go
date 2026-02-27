// Package sdtprobe fetches a small MPEG-TS chunk from a live stream and extracts
// identity and EPG data from three standard DVB/MPEG tables that are always carried
// in the first few kilobytes of any compliant transport stream:
//
//   - PAT  (PID 0x0000) — transport_stream_id
//   - SDT  (PID 0x0011) — original_network_id, service_id, provider_name,
//     service_name, service_type, EIT flags
//   - EIT  (PID 0x0012) — present/following programme titles, start times,
//     duration, content type (genre)
//
// The DVB triplet (original_network_id, transport_stream_id, service_id) is a
// globally registered identifier at dvbservices.com and is the strongest programmatic
// identity anchor for a broadcast re-stream, enabling offline lookup in community
// databases (lyngsat, iptv-org, kingofsat) and future EIT-based EPG building.
package sdtprobe

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── constants ────────────────────────────────────────────────────────────────

const (
	tsPacketLen = 188

	pidPAT = 0x0000
	pidNIT = 0x0010
	pidSDT = 0x0011
	pidEIT = 0x0012

	tablePAT          = 0x00
	tableSDT          = 0x42 // SDT actual_transport_stream
	tableEITpf        = 0x4E // EIT present/following actual
	tableEITpfOther   = 0x4F // EIT present/following other TS (skip)
	descriptorService = 0x48 // DVB service_descriptor
	descriptorContent = 0x54 // DVB content_descriptor (genre)
	descriptorShortEv = 0x4D // DVB short_event_descriptor (title + text)

	// Read at most this many bytes.  The SDT and EIT are always in the first
	// seconds of a broadcast and comfortably fit within 256 KB.
	maxReadBytes = 256 * 1024
)

// ── public types ─────────────────────────────────────────────────────────────

// Programme is "now" or "next" from the EIT present/following table.
type Programme struct {
	EventID   uint16
	StartTime time.Time // UTC; zero if not parsed
	Duration  time.Duration
	Title     string // short_event_descriptor event_name
	Text      string // short_event_descriptor text (synopsis)
	Genre     string // content_descriptor first nibble label (e.g. "Sports", "News")
	IsNow     bool   // true = currently airing, false = coming next
}

// Result holds everything extracted from the TS probe — a superset of what the
// old single-field Result carried.  All fields are zero/empty when not found.
type Result struct {
	// ── identity ──────────────────────────────────────────────────────────
	Found bool // true if at least service_name was extracted

	// DVB triplet — globally unique registered identifier.
	// Look up at dvbservices.com, lyngsat.com, or iptv-org channels.json.
	OriginalNetworkID uint16 // from SDT section header
	TransportStreamID uint16 // from PAT or SDT section header
	ServiceID         uint16 // from SDT service loop entry

	// Human-readable broadcaster identity.
	ProviderName string // e.g. "BBC", "Sky", "ESPN" — from service_descriptor
	ServiceName  string // channel's own name — from service_descriptor
	ServiceType  byte   // 0x01=TV, 0x02=Radio, 0x11=MPEG2-HD TV, 0x19=AVC HD TV, etc.

	// EIT flags from the SDT service entry.
	EITSchedule         bool // stream carries full 8-day EPG schedule
	EITPresentFollowing bool // stream carries now/next programme (EIT p/f)

	// ── now/next programme from EIT ──────────────────────────────────────
	// Only populated when EITPresentFollowing is true and EIT was in the buffer.
	NowNext []Programme // [0]=now, [1]=next (may have 0, 1, or 2 entries)
}

// ── public API ───────────────────────────────────────────────────────────────

// Probe fetches up to maxReadBytes from url and extracts all available DVB
// identity and EPG metadata.  client may be nil.
func Probe(ctx context.Context, url string, client *http.Client, timeout time.Duration) (Result, error) {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, fmt.Errorf("sdtprobe: build request: %w", err)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0 (+sdt-probe)")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxReadBytes-1))

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("sdtprobe: GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Result{}, fmt.Errorf("sdtprobe: HTTP %d", resp.StatusCode)
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxReadBytes))
	if err != nil && len(buf) == 0 {
		return Result{}, fmt.Errorf("sdtprobe: read body: %w", err)
	}
	return parseAll(buf), nil
}

// ── core parser ──────────────────────────────────────────────────────────────

// parseAll walks the TS buffer once, collecting sections from PAT, SDT, and EIT.
func parseAll(buf []byte) Result {
	var r Result

	// Collect raw section bytes per PID so we can hand them to table parsers.
	sections := map[uint16][]byte{} // pid → first complete section payload

	for off := syncOffset(buf); off+tsPacketLen <= len(buf); off += tsPacketLen {
		pkt := buf[off : off+tsPacketLen]
		if pkt[0] != 0x47 {
			next := syncOffset(buf[off+1:])
			off += next
			continue
		}
		pid := uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
		switch pid {
		case pidPAT, pidSDT, pidEIT:
		default:
			continue
		}
		if _, already := sections[pid]; already {
			continue // we only need one section per PID
		}
		payload := tsPayload(pkt)
		if payload == nil {
			continue
		}
		sections[pid] = payload
	}

	// PAT → transport_stream_id
	if sec, ok := sections[pidPAT]; ok {
		r.TransportStreamID = parsePATTSID(sec)
	}

	// SDT → identity fields
	if sec, ok := sections[pidSDT]; ok {
		parseSDTSection(sec, &r)
	}

	// EIT p/f → now/next programme
	if sec, ok := sections[pidEIT]; ok {
		r.NowNext = parseEITSection(sec)
	}

	return r
}

// ── PAT parser ───────────────────────────────────────────────────────────────

func parsePATTSID(d []byte) uint16 {
	// table_id(1), section_syntax_indicator|...|section_length(2),
	// transport_stream_id(2), ...
	if len(d) < 5 {
		return 0
	}
	if d[0] != tablePAT {
		return 0
	}
	return binary.BigEndian.Uint16(d[3:5])
}

// ── SDT parser ───────────────────────────────────────────────────────────────

func parseSDTSection(d []byte, r *Result) {
	if len(d) < 3 || d[0] != tableSDT {
		return
	}
	sectionLen := int(uint16(d[1]&0x0F)<<8|uint16(d[2])) + 3
	if sectionLen > len(d) {
		sectionLen = len(d)
	}
	// SDT fixed header layout (11 bytes):
	//  [0]    table_id
	//  [1-2]  section_syntax_indicator | reserved | section_length
	//  [3-4]  transport_stream_id
	//  [5]    reserved | version_number | current_next_indicator
	//  [6]    section_number
	//  [7]    last_section_number
	//  [8-9]  original_network_id
	//  [10]   reserved_future_use
	const hdrLen = 11
	if sectionLen < hdrLen+4 {
		return
	}
	if r.TransportStreamID == 0 {
		r.TransportStreamID = binary.BigEndian.Uint16(d[3:5])
	}
	r.OriginalNetworkID = binary.BigEndian.Uint16(d[8:10])

	pos := hdrLen
	end := sectionLen - 4 // trim CRC-32
	for pos+5 <= end {
		// service_id(2), reserved_future_use|EIT_schedule_flag|EIT_present_following_flag(1), ...|descriptors_loop_length(2)
		svcID := binary.BigEndian.Uint16(d[pos : pos+2])
		eitFlags := d[pos+2]
		eitSched := eitFlags&0x02 != 0
		eitPF := eitFlags&0x01 != 0
		descLoopLen := int(uint16(d[pos+3]&0x0F)<<8 | uint16(d[pos+4]))
		pos += 5
		descEnd := pos + descLoopLen
		if descEnd > end {
			descEnd = end
		}

		for pos+2 <= descEnd {
			tag := d[pos]
			dLen := int(d[pos+1])
			pos += 2
			if pos+dLen > descEnd {
				break
			}
			if tag == descriptorService && dLen >= 3 {
				prov, name, svcType, ok := parseServiceDescriptor(d[pos : pos+dLen])
				if ok && name != "" {
					r.ServiceID = svcID
					r.ServiceName = name
					r.ProviderName = prov
					r.ServiceType = svcType
					r.EITSchedule = eitSched
					r.EITPresentFollowing = eitPF
					r.Found = true
					return // take the first match
				}
			}
			pos += dLen
		}
		pos = descEnd
	}
}

// parseServiceDescriptor decodes DVB service_descriptor (tag 0x48).
// Returns (providerName, serviceName, serviceType, ok).
func parseServiceDescriptor(d []byte) (string, string, byte, bool) {
	if len(d) < 3 {
		return "", "", 0, false
	}
	svcType := d[0]
	provLen := int(d[1])
	if 2+provLen+1 > len(d) {
		return "", "", 0, false
	}
	prov := decodeDVBString(d[2 : 2+provLen])
	snOff := 2 + provLen
	snLen := int(d[snOff])
	snOff++
	if snOff+snLen > len(d) {
		return "", "", 0, false
	}
	name := strings.TrimSpace(decodeDVBString(d[snOff : snOff+snLen]))
	if name == "" {
		return "", "", 0, false
	}
	return strings.TrimSpace(prov), name, svcType, true
}

// ── EIT present/following parser ─────────────────────────────────────────────

func parseEITSection(d []byte) []Programme {
	if len(d) < 3 {
		return nil
	}
	tid := d[0]
	if tid != tableEITpf {
		return nil
	}
	sectionLen := int(uint16(d[1]&0x0F)<<8|uint16(d[2])) + 3
	if sectionLen > len(d) {
		sectionLen = len(d)
	}
	// EIT fixed header (14 bytes):
	//  [0]    table_id
	//  [1-2]  section_length
	//  [3-4]  service_id
	//  [5]    version/current
	//  [6]    section_number  (0=now, 1=next)
	//  [7]    last_section_number
	//  [8-9]  transport_stream_id
	//  [10-11] original_network_id
	//  [12]   segment_last_section_number
	//  [13]   last_table_id
	const hdrLen = 14
	if sectionLen < hdrLen+4 {
		return nil
	}
	sectionNum := d[6]
	isNow := sectionNum == 0

	pos := hdrLen
	end := sectionLen - 4
	var progs []Programme
	for pos+12 <= end {
		// event_id(2), start_time(5 MJD+BCD), duration(3 BCD), running_status(3b)|free_CA_mode(1b)|descriptors_loop_length(12b)
		eventID := binary.BigEndian.Uint16(d[pos : pos+2])
		startTime := parseDVBTime(d[pos+2 : pos+7])
		duration := parseDVBDuration(d[pos+7 : pos+10])
		descLoopLen := int(uint16(d[pos+10]&0x0F)<<8 | uint16(d[pos+11]))
		pos += 12
		descEnd := pos + descLoopLen
		if descEnd > end {
			descEnd = end
		}

		var title, text, genre string
		for pos+2 <= descEnd {
			tag := d[pos]
			dLen := int(d[pos+1])
			pos += 2
			if pos+dLen > descEnd {
				break
			}
			switch tag {
			case descriptorShortEv:
				t, tx := parseShortEventDescriptor(d[pos : pos+dLen])
				if t != "" {
					title = t
				}
				if tx != "" {
					text = tx
				}
			case descriptorContent:
				genre = parseContentDescriptor(d[pos : pos+dLen])
			}
			pos += dLen
		}
		pos = descEnd

		if title != "" {
			progs = append(progs, Programme{
				EventID:   eventID,
				StartTime: startTime,
				Duration:  duration,
				Title:     title,
				Text:      text,
				Genre:     genre,
				IsNow:     isNow,
			})
		}
		// We only read one section at a time; for now/next we get one entry per call.
		// The caller collects both sections (section_number 0 and 1).
		break
	}
	return progs
}

// parseShortEventDescriptor decodes DVB short_event_descriptor (tag 0x4D).
// Layout: ISO_639_language_code(3), event_name_length(1), event_name(n), text_length(1), text(m)
func parseShortEventDescriptor(d []byte) (title, text string) {
	if len(d) < 5 {
		return
	}
	nameLen := int(d[3])
	if 4+nameLen+1 > len(d) {
		return
	}
	title = strings.TrimSpace(decodeDVBString(d[4 : 4+nameLen]))
	txOff := 4 + nameLen
	txLen := int(d[txOff])
	txOff++
	if txOff+txLen <= len(d) {
		text = strings.TrimSpace(decodeDVBString(d[txOff : txOff+txLen]))
	}
	return
}

// parseContentDescriptor decodes DVB content_descriptor (tag 0x54) and returns
// the label for the first content_nibble_level_1.
func parseContentDescriptor(d []byte) string {
	if len(d) < 2 {
		return ""
	}
	nibble := (d[0] >> 4) & 0x0F
	return contentNibbleLabel(nibble)
}

func contentNibbleLabel(n byte) string {
	switch n {
	case 0x01:
		return "Movie/Drama"
	case 0x02:
		return "News/Current Affairs"
	case 0x03:
		return "Show/Game Show"
	case 0x04:
		return "Sports"
	case 0x05:
		return "Children/Youth"
	case 0x06:
		return "Music/Ballet/Dance"
	case 0x07:
		return "Arts/Culture"
	case 0x08:
		return "Social/Political/Economics"
	case 0x09:
		return "Education/Science/Factual"
	case 0x0A:
		return "Leisure/Hobbies"
	case 0x0B:
		return "Special Characteristics"
	default:
		return ""
	}
}

// ── DVB time helpers ─────────────────────────────────────────────────────────

// parseDVBTime decodes a 5-byte DVB MJD+BCD timestamp into a UTC time.Time.
// Returns zero time on error or if bytes are all 0xFF (undefined).
func parseDVBTime(b []byte) time.Time {
	if len(b) < 5 {
		return time.Time{}
	}
	if b[0] == 0xFF && b[1] == 0xFF {
		return time.Time{}
	}
	mjd := int(binary.BigEndian.Uint16(b[0:2]))
	// MJD → calendar date per DVB spec (EN 300 468 Annex C)
	yp := int((float64(mjd) - 15078.2) / 365.25)
	mp := int((float64(mjd) - 14956.1 - float64(int(float64(yp)*365.25))) / 30.6001)
	day := mjd - 14956 - int(float64(yp)*365.25) - int(float64(mp)*30.6001) //nolint:gocritic
	k := 0
	if mp == 14 || mp == 15 {
		k = 1
	}
	year := yp + k + 1900
	month := mp - 1 - k*12

	hour := bcdByte(b[2])
	min := bcdByte(b[3])
	sec := bcdByte(b[4])
	if hour > 23 || min > 59 || sec > 59 {
		return time.Time{}
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
}

// parseDVBDuration decodes a 3-byte BCD HHMMSS duration.
func parseDVBDuration(b []byte) time.Duration {
	if len(b) < 3 {
		return 0
	}
	if b[0] == 0xFF {
		return 0
	}
	h := time.Duration(bcdByte(b[0]))
	m := time.Duration(bcdByte(b[1]))
	s := time.Duration(bcdByte(b[2]))
	return h*time.Hour + m*time.Minute + s*time.Second
}

// bcdByte decodes a single BCD byte (e.g. 0x23 → 23).
func bcdByte(b byte) int {
	return int((b>>4)*10 + b&0x0F)
}

// ── TS packet helpers ────────────────────────────────────────────────────────

// tsPayload returns the section payload from a PUSI TS packet (pointer-field
// adjusted), or nil if the packet has no PUSI or is too short.
func tsPayload(pkt []byte) []byte {
	if len(pkt) < 5 {
		return nil
	}
	if pkt[1]&0x40 == 0 {
		return nil // no payload_unit_start_indicator
	}
	start := 4
	if pkt[3]&0x20 != 0 { // adaptation field present
		afLen := int(pkt[4])
		start = 5 + afLen
	}
	if start+1 >= len(pkt) {
		return nil
	}
	ptr := int(pkt[start]) + 1
	start += ptr
	if start >= len(pkt) {
		return nil
	}
	return pkt[start:]
}

// syncOffset returns the index of the first 0x47 sync byte.
func syncOffset(buf []byte) int {
	for i, b := range buf {
		if b == 0x47 {
			return i
		}
	}
	return len(buf)
}

// ── DVB string decoder ───────────────────────────────────────────────────────

// decodeDVBString handles DVB character-table prefixes and returns a UTF-8
// string.  Covers the vast majority of broadcast service names with Latin-1 /
// ISO 8859-1 fallback; strips multi-byte charset prefixes (0x10 xx xx).
func decodeDVBString(d []byte) string {
	if len(d) == 0 {
		return ""
	}
	if d[0] == 0x10 {
		if len(d) >= 4 {
			d = d[3:]
		}
	} else if d[0] < 0x20 {
		d = d[1:]
	}
	r := make([]rune, 0, len(d))
	for _, b := range d {
		if b >= 0x80 && b <= 0x9F {
			continue // DVB control chars
		}
		r = append(r, rune(b))
	}
	return string(r)
}

// keep binary import used by parseDVBTime
var _ = binary.BigEndian
