package tuner

import "testing"

func testTSPacket(payload []byte) []byte {
	p := make([]byte, 188)
	p[0] = 0x47
	p[1] = 0x00
	p[2] = 0x00
	// Use adaptation+payload so short synthetic payloads do not get padded inside the
	// payload region (which would break cross-packet boundary tests).
	p[3] = 0x30
	alen := len(p) - 5 - len(payload)
	if alen < 0 {
		alen = 0
	}
	p[4] = byte(alen)
	if alen > 0 {
		// First adaptation byte is flags; remainder can be stuffing.
		p[5] = 0x00
		for i := 6; i < 5+alen; i++ {
			p[i] = 0xff
		}
	}
	start := 5 + alen
	n := copy(p[start:], payload)
	for i := start + n; i < len(p); i++ {
		p[i] = 0x00
	}
	return p
}

func TestLooksLikeGoodTSStartDetectsMaskedIDR(t *testing.T) {
	var buf []byte
	for i := 0; i < 8; i++ {
		payload := []byte{byte(i)}
		switch i {
		case 2:
			// AAC ADTS syncword
			payload = []byte{0x11, 0xff, 0xf1, 0x50, 0x80}
		case 5:
			// H264 Annex B IDR with non-0x65 NAL header (NAL type 5, lower ref bits).
			payload = []byte{0x00, 0x00, 0x01, 0x45, 0x88, 0x84}
		}
		buf = append(buf, testTSPacket(payload)...)
	}
	st := looksLikeGoodTSStart(buf)
	if !st.HasAAC {
		t.Fatalf("expected AAC detection")
	}
	if !st.HasIDR {
		t.Fatalf("expected IDR detection for masked nal header")
	}
	if st.TSLikePackets < 8 {
		t.Fatalf("expected TSLikePackets>=8, got %d", st.TSLikePackets)
	}
}

func TestLooksLikeGoodTSStartDetectsSplitIDRStartCodeAcrossPackets(t *testing.T) {
	var buf []byte
	for i := 0; i < 8; i++ {
		payload := []byte{byte(i), 0xaa, 0xbb}
		switch i {
		case 1:
			payload = []byte{0xff, 0xf1, 0x50, 0x80}
		case 4:
			// End with partial start code.
			payload = []byte{0x99, 0x88, 0x00, 0x00}
		case 5:
			// Continue across the next packet payload.
			payload = []byte{0x01, 0x65, 0x88, 0x84, 0x21}
		}
		buf = append(buf, testTSPacket(payload)...)
	}
	st := looksLikeGoodTSStart(buf)
	if !st.HasAAC {
		t.Fatalf("expected AAC detection")
	}
	if !st.HasIDR {
		t.Fatalf("expected IDR detection across packet boundary")
	}
}

func TestContainsHEVCIRAPAnnexB(t *testing.T) {
	// NAL type 19 (IDR_W_RADL): nal_unit_type = (0x26 >> 1) & 0x3F = 19
	nal := []byte{0x00, 0x00, 0x01, 0x26, 0x01, 0x02}
	if !containsHEVCIRAPAnnexB(nal) {
		t.Fatal("expected HEVC IRAP detection for 3-byte start")
	}
	nal4 := []byte{0x00, 0x00, 0x00, 0x01, 0x2A, 0x01} // type 21 CRA: (0x2a>>1)=21
	if !containsHEVCIRAPAnnexB(nal4) {
		t.Fatal("expected HEVC CRA for 4-byte start")
	}
	// VPS (32): (0x40>>1)&0x3f = 32 — not IRAP
	vps := []byte{0x00, 0x00, 0x01, 0x40, 0x01}
	if containsHEVCIRAPAnnexB(vps) {
		t.Fatal("did not expect VPS to count as IRAP")
	}
}

func TestLooksLikeGoodTSStartDetectsHEVCIRAP(t *testing.T) {
	var buf []byte
	for i := 0; i < 8; i++ {
		payload := []byte{byte(i)}
		switch i {
		case 2:
			payload = []byte{0x11, 0xff, 0xf1, 0x50, 0x80}
		case 5:
			payload = []byte{0x00, 0x00, 0x01, 0x26, 0x01, 0x02} // HEVC IRAP type 19
		}
		buf = append(buf, testTSPacket(payload)...)
	}
	st := looksLikeGoodTSStart(buf)
	if !st.HasAAC || !st.HasIDR {
		t.Fatalf("want aac+hevc irap, got idr=%v aac=%v", st.HasIDR, st.HasAAC)
	}
}

func TestLooksLikeGoodTSStartDetectsSplitHEVCStartCodeAcrossPackets(t *testing.T) {
	var buf []byte
	for i := 0; i < 8; i++ {
		payload := []byte{byte(i), 0xaa}
		switch i {
		case 1:
			payload = []byte{0xff, 0xf1, 0x50, 0x80}
		case 4:
			payload = []byte{0x99, 0x88, 0x00, 0x00}
		case 5:
			payload = []byte{0x01, 0x26, 0x01, 0x02}
		}
		buf = append(buf, testTSPacket(payload)...)
	}
	st := looksLikeGoodTSStart(buf)
	if !st.HasAAC || !st.HasIDR {
		t.Fatalf("expected split start code IRAP+aac, idr=%v aac=%v", st.HasIDR, st.HasAAC)
	}
}

func TestTrimTSHeadToMaxBytesKeepsTailAndSync(t *testing.T) {
	var prefix []byte
	for i := 0; i < 25; i++ {
		prefix = append(prefix, testTSPacket([]byte{byte(i)})...)
	}
	var goodTail []byte
	for i := 0; i < 8; i++ {
		payload := []byte{byte(i)}
		switch i {
		case 2:
			payload = []byte{0x11, 0xff, 0xf1, 0x50, 0x80}
		case 5:
			payload = []byte{0x00, 0x00, 0x01, 0x45, 0x88, 0x84}
		}
		goodTail = append(goodTail, testTSPacket(payload)...)
	}
	buf := append(prefix, goodTail...)
	maxB := tsPacketSize * 10
	out := trimTSHeadToMaxBytes(buf, maxB)
	if len(out) > maxB {
		t.Fatalf("len=%d want <=%d", len(out), maxB)
	}
	if out[0] != 0x47 {
		t.Fatalf("expected sync 0x47, got %02x", out[0])
	}
	st := looksLikeGoodTSStart(out)
	if !st.HasIDR || !st.HasAAC {
		t.Fatalf("lost IDR/AAC after trim: idr=%v aac=%v", st.HasIDR, st.HasAAC)
	}
}
