package sdtprobe

import (
	"encoding/binary"
	"testing"
	"time"
)

// ── TS / section builders ─────────────────────────────────────────────────────

// buildTSPacket returns a 188-byte TS packet with PUSI=1, no adaptation field.
func buildTSPacket(pid uint16, payload []byte) []byte {
	pkt := make([]byte, tsPacketLen)
	pkt[0] = 0x47
	pkt[1] = byte(0x40 | (pid>>8)&0x1F)
	pkt[2] = byte(pid & 0xFF)
	pkt[3] = 0x10 // payload only
	pkt[4] = 0x00 // pointer_field = 0
	copy(pkt[5:], payload)
	return pkt
}

// buildPATSection returns a minimal PAT section for tsid.
func buildPATSection(tsid uint16) []byte {
	sec := make([]byte, 13) // header(8) + one program entry(4) + CRC(4) - 3
	sec[0] = tablePAT
	sectionLen := len(sec) - 3
	sec[1] = 0xF0 | byte(sectionLen>>8)
	sec[2] = byte(sectionLen & 0xFF)
	binary.BigEndian.PutUint16(sec[3:], tsid)
	sec[5] = 0xC1 // version=0, current=1
	sec[6] = 0x00
	sec[7] = 0x00
	// program 1 → PMT PID 0x100
	sec[8] = 0x00
	sec[9] = 0x01
	sec[10] = 0xE1
	sec[11] = 0x00
	// dummy CRC
	binary.BigEndian.PutUint32(sec[len(sec)-4:], 0xDEADBEEF)
	return sec
}

// buildSDTSection constructs a DVB SDT section with one service entry.
func buildSDTSection(onid, tsid, svcID uint16, svcType byte, eitSched, eitPF bool, providerName, serviceName string) []byte {
	provBytes := []byte(providerName)
	svcBytes := []byte(serviceName)
	descPayload := []byte{svcType, byte(len(provBytes))}
	descPayload = append(descPayload, provBytes...)
	descPayload = append(descPayload, byte(len(svcBytes)))
	descPayload = append(descPayload, svcBytes...)
	descriptor := append([]byte{descriptorService, byte(len(descPayload))}, descPayload...)

	var eitByte byte
	if eitSched {
		eitByte |= 0x02
	}
	if eitPF {
		eitByte |= 0x01
	}
	descLoopLen := len(descriptor)
	entry := []byte{
		byte(svcID >> 8), byte(svcID),
		0xFC | eitByte, // reserved(6) | eit flags
		0xF0 | byte(descLoopLen>>8&0x0F), byte(descLoopLen),
	}
	entry = append(entry, descriptor...)

	sec := make([]byte, 11)
	sec[0] = tableSDT
	payloadLen := len(entry) + 4
	sectionLen := 11 - 3 + payloadLen
	sec[1] = 0xF0 | byte(sectionLen>>8)
	sec[2] = byte(sectionLen & 0xFF)
	binary.BigEndian.PutUint16(sec[3:], tsid)
	sec[5] = 0xC1
	sec[6] = 0x00
	sec[7] = 0x00
	binary.BigEndian.PutUint16(sec[8:], onid)
	sec[10] = 0xFF
	sec = append(sec, entry...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, 0xDEADBEEF)
	return append(sec, crc...)
}

// buildShortEventDescriptor builds a DVB short_event_descriptor.
func buildShortEventDescriptor(lang, title, text string) []byte {
	b := []byte(lang[:3])
	b = append(b, byte(len(title)))
	b = append(b, []byte(title)...)
	b = append(b, byte(len(text)))
	b = append(b, []byte(text)...)
	return append([]byte{descriptorShortEv, byte(len(b))}, b...)
}

// buildContentDescriptor builds a DVB content_descriptor with one nibble pair.
func buildContentDescriptor(nibble1 byte) []byte {
	return []byte{descriptorContent, 2, (nibble1 << 4) | 0x00, 0x00}
}

// buildEITSection builds a minimal EIT present/following section.
// sectionNum=0 → "now",  sectionNum=1 → "next".
func buildEITSection(sectionNum byte, eventID uint16, title, text, genre string) []byte {
	// start time: 2026-01-01 20:00:00 UTC in MJD+BCD
	// MJD for 2026-01-01 = 61041 = 0xEE71
	startTime := []byte{0xEE, 0x71, 0x20, 0x00, 0x00}
	// duration: 01:30:00 BCD
	duration := []byte{0x01, 0x30, 0x00}

	var descs []byte
	descs = append(descs, buildShortEventDescriptor("eng", title, text)...)
	if genre != "" {
		nibble := genreNibble(genre)
		descs = append(descs, buildContentDescriptor(nibble)...)
	}

	descLoopLen := len(descs)
	// event entry: event_id(2) + start_time(5) + duration(3) + running_status+free_CA+loop_len(2)
	entry := []byte{
		byte(eventID >> 8), byte(eventID),
	}
	entry = append(entry, startTime...)
	entry = append(entry, duration...)
	entry = append(entry, 0xA0|byte(descLoopLen>>8&0x0F), byte(descLoopLen)) // running=5(running)
	entry = append(entry, descs...)

	// EIT header: 14 bytes
	sec := make([]byte, 14)
	sec[0] = tableEITpf
	payloadLen := len(entry) + 4
	sectionLen := 14 - 3 + payloadLen
	sec[1] = 0xF0 | byte(sectionLen>>8)
	sec[2] = byte(sectionLen & 0xFF)
	sec[3] = 0x00
	sec[4] = 0x01 // service_id = 1
	sec[5] = 0xC1
	sec[6] = sectionNum
	sec[7] = 0x01 // last_section_number
	sec[8] = 0x00
	sec[9] = 0x01 // tsid = 1
	sec[10] = 0x00
	sec[11] = 0x01 // onid = 1
	sec[12] = 0x01
	sec[13] = tableEITpf
	sec = append(sec, entry...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, 0xDEADBEEF)
	return append(sec, crc...)
}

func genreNibble(genre string) byte {
	switch genre {
	case "Sports":
		return 0x04
	case "News/Current Affairs":
		return 0x02
	case "Movie/Drama":
		return 0x01
	default:
		return 0x00
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestParseAll_FullBundle(t *testing.T) {
	var buf []byte
	// PAT: tsid=0x1234
	buf = append(buf, buildTSPacket(pidPAT, buildPATSection(0x1234))...)
	// SDT: onid=0x233D (Sky UK), tsid=0x1234, svcid=0x0042, type=0x19 (AVC HD TV)
	//      eit_schedule=true, eit_pf=true, provider="Sky", service="Sky Sports 1"
	buf = append(buf, buildTSPacket(pidSDT, buildSDTSection(0x233D, 0x1234, 0x0042, 0x19, true, true, "Sky", "Sky Sports 1"))...)
	// EIT now (section_number=0)
	buf = append(buf, buildTSPacket(pidEIT, buildEITSection(0, 0xABCD, "Premier League Football", "Live coverage", "Sports"))...)

	r := parseAll(buf)

	if !r.Found {
		t.Fatal("expected Found=true")
	}
	if r.TransportStreamID != 0x1234 {
		t.Errorf("TransportStreamID: want 0x1234, got 0x%04x", r.TransportStreamID)
	}
	if r.OriginalNetworkID != 0x233D {
		t.Errorf("OriginalNetworkID: want 0x233D, got 0x%04x", r.OriginalNetworkID)
	}
	if r.ServiceID != 0x0042 {
		t.Errorf("ServiceID: want 0x0042, got 0x%04x", r.ServiceID)
	}
	if r.ProviderName != "Sky" {
		t.Errorf("ProviderName: want %q, got %q", "Sky", r.ProviderName)
	}
	if r.ServiceName != "Sky Sports 1" {
		t.Errorf("ServiceName: want %q, got %q", "Sky Sports 1", r.ServiceName)
	}
	if r.ServiceType != 0x19 {
		t.Errorf("ServiceType: want 0x19, got 0x%02x", r.ServiceType)
	}
	if !r.EITSchedule {
		t.Error("EITSchedule: want true")
	}
	if !r.EITPresentFollowing {
		t.Error("EITPresentFollowing: want true")
	}
	// EIT now
	if len(r.NowNext) == 0 {
		t.Fatal("NowNext: want at least 1 programme")
	}
	now := r.NowNext[0]
	if !now.IsNow {
		t.Error("NowNext[0].IsNow: want true")
	}
	if now.Title != "Premier League Football" {
		t.Errorf("NowNext[0].Title: want %q, got %q", "Premier League Football", now.Title)
	}
	if now.Text != "Live coverage" {
		t.Errorf("NowNext[0].Text: want %q, got %q", "Live coverage", now.Text)
	}
	if now.Genre != "Sports" {
		t.Errorf("NowNext[0].Genre: want %q, got %q", "Sports", now.Genre)
	}
}

func TestParseAll_SDTOnly(t *testing.T) {
	// No PAT, no EIT — just SDT.
	buf := buildTSPacket(pidSDT, buildSDTSection(0x0001, 0x0002, 0x0003, 0x01, false, false, "BBC", "BBC ONE"))
	r := parseAll(buf)
	if !r.Found {
		t.Fatal("expected Found=true")
	}
	if r.ServiceName != "BBC ONE" {
		t.Errorf("got %q", r.ServiceName)
	}
	if r.ProviderName != "BBC" {
		t.Errorf("got provider %q", r.ProviderName)
	}
	if r.OriginalNetworkID != 0x0001 {
		t.Errorf("onid: got 0x%04x", r.OriginalNetworkID)
	}
	// tsid should come from SDT when no PAT present
	if r.TransportStreamID != 0x0002 {
		t.Errorf("tsid: got 0x%04x", r.TransportStreamID)
	}
	if r.EITSchedule || r.EITPresentFollowing {
		t.Error("EIT flags should both be false")
	}
}

func TestParseAll_NoSDT(t *testing.T) {
	// Only PAT — no SDT, no EIT.
	buf := buildTSPacket(pidPAT, buildPATSection(0x9999))
	r := parseAll(buf)
	if r.Found {
		t.Error("expected Found=false when no SDT")
	}
	if r.TransportStreamID != 0x9999 {
		t.Errorf("tsid from PAT: want 0x9999, got 0x%04x", r.TransportStreamID)
	}
}

func TestParseAll_EITNowAndNext(t *testing.T) {
	var buf []byte
	buf = append(buf, buildTSPacket(pidSDT, buildSDTSection(0x01, 0x01, 0x01, 0x01, false, true, "", "TestCh"))...)
	// Build two separate EIT packets — the parser takes the first one it finds on pid 0x0012.
	// We test that section_number=0 → IsNow=true.
	buf = append(buf, buildTSPacket(pidEIT, buildEITSection(0, 1, "Now Show", "", ""))...)
	r := parseAll(buf)
	if len(r.NowNext) == 0 {
		t.Fatal("expected at least one programme")
	}
	if !r.NowNext[0].IsNow {
		t.Error("section 0 should be IsNow=true")
	}
	if r.NowNext[0].Title != "Now Show" {
		t.Errorf("got %q", r.NowNext[0].Title)
	}
}

func TestParseDVBTime_Valid(t *testing.T) {
	// 2026-01-01 20:00:00 UTC: MJD=61041=0xEE71, H=0x20, M=0x00, S=0x00
	b := []byte{0xEE, 0x71, 0x20, 0x00, 0x00}
	got := parseDVBTime(b)
	want := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestParseDVBTime_Undefined(t *testing.T) {
	b := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !parseDVBTime(b).IsZero() {
		t.Error("expected zero time for 0xFFFF... timestamp")
	}
}

func TestParseDVBDuration(t *testing.T) {
	// 01:30:00 BCD
	d := parseDVBDuration([]byte{0x01, 0x30, 0x00})
	want := 90 * time.Minute
	if d != want {
		t.Errorf("want %v, got %v", want, d)
	}
}

func TestDecodeDVBString_Latin1(t *testing.T) {
	s := decodeDVBString([]byte("HD caf\xe9"))
	if s != "HD café" {
		t.Errorf("got %q", s)
	}
}

func TestDecodeDVBString_WithCharsetPrefix(t *testing.T) {
	s := decodeDVBString(append([]byte{0x05}, "TRT 1"...))
	if s != "TRT 1" {
		t.Errorf("got %q", s)
	}
}

func TestSyncOffset(t *testing.T) {
	buf := []byte{0x00, 0x01, 0x47, 0x02}
	if got := syncOffset(buf); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
	if got := syncOffset([]byte{0x00, 0x01}); got != 2 {
		t.Errorf("expected len(buf)=2, got %d", got)
	}
}

func TestContentNibbleLabel(t *testing.T) {
	cases := []struct {
		nibble byte
		want   string
	}{
		{0x04, "Sports"},
		{0x02, "News/Current Affairs"},
		{0x01, "Movie/Drama"},
		{0x0F, ""},
	}
	for _, tc := range cases {
		if got := contentNibbleLabel(tc.nibble); got != tc.want {
			t.Errorf("nibble=0x%02x: want %q, got %q", tc.nibble, tc.want, got)
		}
	}
}
