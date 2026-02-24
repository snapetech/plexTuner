package tuner

import (
	"testing"
)

// TestMpegTSCRC32_empty verifies that the CRC of an empty input is the init value (no bytes processed).
func TestMpegTSCRC32_empty(t *testing.T) {
	if got := mpegTSCRC32(nil); got != 0xFFFFFFFF {
		t.Errorf("mpegTSCRC32(nil) = 0x%08X, want 0xFFFFFFFF", got)
	}
	if got := mpegTSCRC32([]byte{}); got != 0xFFFFFFFF {
		t.Errorf("mpegTSCRC32([]) = 0x%08X, want 0xFFFFFFFF", got)
	}
}

// TestMpegTSCRC32_stable verifies that the same input always produces the same CRC.
func TestMpegTSCRC32_stable(t *testing.T) {
	data := []byte{0x00, 0xB0, 0x0D, 0x00, 0x01, 0xC1, 0x00, 0x00, 0x00, 0x01, 0xE0, 0x10}
	a := mpegTSCRC32(data)
	b := mpegTSCRC32(data)
	if a != b {
		t.Errorf("non-deterministic CRC: 0x%08X != 0x%08X", a, b)
	}
}

// TestMpegTSCRC32_notTrivial verifies the CRC is not the init value for non-empty data
// and that different inputs produce different CRCs.
func TestMpegTSCRC32_notTrivial(t *testing.T) {
	crc1 := mpegTSCRC32([]byte{0x00})
	crc2 := mpegTSCRC32([]byte{0x01})
	if crc1 == 0xFFFFFFFF {
		t.Errorf("CRC of single-byte input is unexpectedly 0xFFFFFFFF")
	}
	if crc1 == crc2 {
		t.Errorf("CRC(0x00) == CRC(0x01) = 0x%08X — hash collision on trivial inputs", crc1)
	}
}

// TestBuildPATPacket_size verifies the packet is exactly 188 bytes (one MPEG-TS packet).
func TestBuildPATPacket_size(t *testing.T) {
	pkt := buildPATPacket(0)
	if len(pkt) != 188 {
		t.Errorf("PAT packet size = %d, want 188", len(pkt))
	}
}

// TestBuildPMTPacket_size verifies the packet is exactly 188 bytes (one MPEG-TS packet).
func TestBuildPMTPacket_size(t *testing.T) {
	pkt := buildPMTPacket(0)
	if len(pkt) != 188 {
		t.Errorf("PMT packet size = %d, want 188", len(pkt))
	}
}

// TestBuildPATPacket_structure verifies the full PAT packet layout for several
// continuity counter values.
func TestBuildPATPacket_structure(t *testing.T) {
	for _, cc := range []uint8{0, 5, 15} {
		pkt := buildPATPacket(cc)

		// TS header
		if pkt[0] != 0x47 {
			t.Errorf("cc=%d: sync byte = 0x%02X, want 0x47", cc, pkt[0])
		}
		// PUSI=1, PID=0x0000 → pkt[1]=0x40, pkt[2]=0x00
		if pkt[1] != 0x40 {
			t.Errorf("cc=%d: pkt[1] = 0x%02X, want 0x40 (PUSI=1, PID high=0)", cc, pkt[1])
		}
		if pkt[2] != 0x00 {
			t.Errorf("cc=%d: pkt[2] = 0x%02X, want 0x00 (PID low=0)", cc, pkt[2])
		}
		// adaptation_field_control=0b01 (payload only), continuity_counter=cc
		wantPkt3 := byte(0x10 | (cc & 0x0F))
		if pkt[3] != wantPkt3 {
			t.Errorf("cc=%d: pkt[3] = 0x%02X, want 0x%02X", cc, pkt[3], wantPkt3)
		}
		// pointer_field
		if pkt[4] != 0x00 {
			t.Errorf("cc=%d: pointer_field = 0x%02X, want 0x00", cc, pkt[4])
		}

		// PSI section (starts at pkt[5])
		s := pkt[5:]
		if s[0] != 0x00 {
			t.Errorf("cc=%d: table_id = 0x%02X, want 0x00 (PAT)", cc, s[0])
		}
		// section_length in s[1..2]: low 12 bits of big-endian; s[2] alone for length ≤255
		sectionLen := int(s[1]&0x0F)<<8 | int(s[2])
		if sectionLen != 13 {
			t.Errorf("cc=%d: section_length = %d, want 13", cc, sectionLen)
		}
		// transport_stream_id = 1
		tsid := int(s[3])<<8 | int(s[4])
		if tsid != 1 {
			t.Errorf("cc=%d: transport_stream_id = %d, want 1", cc, tsid)
		}
		// program_number = 1 (at s[8..9])
		progNum := int(s[8])<<8 | int(s[9])
		if progNum != 1 {
			t.Errorf("cc=%d: program_number = %d, want 1", cc, progNum)
		}
		// PMT PID (at s[10..11], low 13 bits)
		pmtPID := int(s[10]&0x1F)<<8 | int(s[11])
		if pmtPID != patPMTKeepPMTPID {
			t.Errorf("cc=%d: PMT PID = 0x%04X, want 0x%04X", cc, pmtPID, patPMTKeepPMTPID)
		}

		// CRC-32: computed over pkt[5..16] (12 bytes), stored at s[12..15]
		wantCRC := mpegTSCRC32(pkt[5:17])
		gotCRC := uint32(s[12])<<24 | uint32(s[13])<<16 | uint32(s[14])<<8 | uint32(s[15])
		if gotCRC != wantCRC {
			t.Errorf("cc=%d: PAT CRC = 0x%08X, want 0x%08X", cc, gotCRC, wantCRC)
		}

		// Remainder must be 0xFF padding
		for i := 21; i < 188; i++ {
			if pkt[i] != 0xFF {
				t.Errorf("cc=%d: pkt[%d] = 0x%02X, want 0xFF (padding)", cc, i, pkt[i])
				break
			}
		}
	}
}

// TestBuildPMTPacket_structure verifies the full PMT packet layout for several
// continuity counter values.
func TestBuildPMTPacket_structure(t *testing.T) {
	for _, cc := range []uint8{0, 3, 15} {
		pkt := buildPMTPacket(cc)

		// TS header
		if pkt[0] != 0x47 {
			t.Errorf("cc=%d: sync byte = 0x%02X, want 0x47", cc, pkt[0])
		}
		// PUSI=1, PID=patPMTKeepPMTPID (0x1000)
		wantPID1 := byte(0x40 | ((patPMTKeepPMTPID >> 8) & 0x1F))
		wantPID2 := byte(patPMTKeepPMTPID & 0xFF)
		if pkt[1] != wantPID1 || pkt[2] != wantPID2 {
			t.Errorf("cc=%d: PID bytes = 0x%02X 0x%02X, want 0x%02X 0x%02X",
				cc, pkt[1], pkt[2], wantPID1, wantPID2)
		}
		wantPkt3 := byte(0x10 | (cc & 0x0F))
		if pkt[3] != wantPkt3 {
			t.Errorf("cc=%d: pkt[3] = 0x%02X, want 0x%02X", cc, pkt[3], wantPkt3)
		}
		if pkt[4] != 0x00 {
			t.Errorf("cc=%d: pointer_field = 0x%02X, want 0x00", cc, pkt[4])
		}

		// PSI section
		s := pkt[5:]
		if s[0] != 0x02 {
			t.Errorf("cc=%d: table_id = 0x%02X, want 0x02 (PMT)", cc, s[0])
		}
		sectionLen := int(s[1]&0x0F)<<8 | int(s[2])
		if sectionLen != 23 {
			t.Errorf("cc=%d: section_length = %d, want 23", cc, sectionLen)
		}
		progNum := int(s[3])<<8 | int(s[4])
		if progNum != 1 {
			t.Errorf("cc=%d: program_number = %d, want 1", cc, progNum)
		}
		// PCR_PID = video PID (at s[8..9], low 13 bits)
		pcrPID := int(s[8]&0x1F)<<8 | int(s[9])
		if pcrPID != patPMTKeepVideoPID {
			t.Errorf("cc=%d: PCR PID = 0x%04X, want 0x%04X", cc, pcrPID, patPMTKeepVideoPID)
		}
		// program_info_length = 0 (at s[10..11])
		progInfoLen := int(s[10]&0x0F)<<8 | int(s[11])
		if progInfoLen != 0 {
			t.Errorf("cc=%d: program_info_length = %d, want 0", cc, progInfoLen)
		}

		// Video stream entry (at s[12..16])
		if s[12] != 0x1B {
			t.Errorf("cc=%d: video stream_type = 0x%02X, want 0x1B (H.264)", cc, s[12])
		}
		videoPID := int(s[13]&0x1F)<<8 | int(s[14])
		if videoPID != patPMTKeepVideoPID {
			t.Errorf("cc=%d: video PID = 0x%04X, want 0x%04X", cc, videoPID, patPMTKeepVideoPID)
		}
		videoInfoLen := int(s[15]&0x0F)<<8 | int(s[16])
		if videoInfoLen != 0 {
			t.Errorf("cc=%d: video ES_info_length = %d, want 0", cc, videoInfoLen)
		}

		// Audio stream entry (at s[17..21])
		if s[17] != 0x0F {
			t.Errorf("cc=%d: audio stream_type = 0x%02X, want 0x0F (AAC)", cc, s[17])
		}
		audioPID := int(s[18]&0x1F)<<8 | int(s[19])
		if audioPID != patPMTKeepAudioPID {
			t.Errorf("cc=%d: audio PID = 0x%04X, want 0x%04X", cc, audioPID, patPMTKeepAudioPID)
		}
		audioInfoLen := int(s[20]&0x0F)<<8 | int(s[21])
		if audioInfoLen != 0 {
			t.Errorf("cc=%d: audio ES_info_length = %d, want 0", cc, audioInfoLen)
		}

		// CRC-32: computed over pkt[5..26] (22 bytes), stored at s[22..25]
		wantCRC := mpegTSCRC32(pkt[5:27])
		gotCRC := uint32(s[22])<<24 | uint32(s[23])<<16 | uint32(s[24])<<8 | uint32(s[25])
		if gotCRC != wantCRC {
			t.Errorf("cc=%d: PMT CRC = 0x%08X, want 0x%08X", cc, gotCRC, wantCRC)
		}

		// Padding
		for i := 31; i < 188; i++ {
			if pkt[i] != 0xFF {
				t.Errorf("cc=%d: pkt[%d] = 0x%02X, want 0xFF (padding)", cc, i, pkt[i])
				break
			}
		}
	}
}

// TestBuildPATPacket_ccMask verifies that only the low 4 bits of cc are used.
func TestBuildPATPacket_ccMask(t *testing.T) {
	pkt := buildPATPacket(0xFF)
	if pkt[3]&0x0F != 0x0F {
		t.Errorf("cc=0xFF: low nibble of pkt[3] = 0x%X, want 0xF", pkt[3]&0x0F)
	}
	if pkt[3]&0xF0 != 0x10 {
		t.Errorf("cc=0xFF: high nibble of pkt[3] = 0x%X, want 0x1 (payload-only flag)", pkt[3]>>4)
	}
}

// TestBuildPMTPacket_ccMask verifies that only the low 4 bits of cc are used.
func TestBuildPMTPacket_ccMask(t *testing.T) {
	pkt := buildPMTPacket(0xFF)
	if pkt[3]&0x0F != 0x0F {
		t.Errorf("cc=0xFF: low nibble of pkt[3] = 0x%X, want 0xF", pkt[3]&0x0F)
	}
	if pkt[3]&0xF0 != 0x10 {
		t.Errorf("cc=0xFF: high nibble of pkt[3] = 0x%X, want 0x1 (payload-only flag)", pkt[3]>>4)
	}
}

// TestBuildPATPacket_crcChangesWithData verifies the CRC is content-sensitive
// (a different section_length would yield a different CRC).
func TestBuildPATPacket_crcChangesWithData(t *testing.T) {
	// Two distinct data patterns to confirm CRC is sensitive to content.
	data1 := []byte{0x00, 0xB0, 0x0D, 0x00, 0x01, 0xC1, 0x00, 0x00, 0x00, 0x01, 0xE0, 0x10}
	data2 := []byte{0x00, 0xB0, 0x0D, 0x00, 0x02, 0xC1, 0x00, 0x00, 0x00, 0x01, 0xE0, 0x10} // tsid=2
	if mpegTSCRC32(data1) == mpegTSCRC32(data2) {
		t.Error("CRC of different sections unexpectedly equal")
	}
}
