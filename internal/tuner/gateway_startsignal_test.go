package tuner

import "testing"

func testTSPacket(payload []byte) []byte {
	p := make([]byte, 188)
	p[0] = 0x47
	p[1] = 0x00
	p[2] = 0x00
	p[3] = 0x10 // payload only
	n := copy(p[4:], payload)
	for i := 4 + n; i < len(p); i++ {
		p[i] = 0xff
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
