package tuner

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// MPEG-TS PSI program-info keepalive: PAT + PMT packet builders.
//
// PID values match ffmpeg mpegts muxer defaults (mpegts_pmt_start_pid=0x1000,
// mpegts_start_pid=0x100) so the keepalive packets declare the same program
// structure that the bootstrap and real-stream packets will use.
const (
	patPMTKeepPMTPID   = 0x1000 // ffmpeg default first PMT PID
	patPMTKeepVideoPID = 0x0100 // ffmpeg default video elementary stream PID
	patPMTKeepAudioPID = 0x0101 // ffmpeg default audio elementary stream PID
)

// mpegTSCRC32 computes the MPEG-2 section CRC-32 (polynomial 0x04C11DB7,
// init 0xFFFFFFFF, MSB-first, no bit reflection, no final XOR) used in PAT/PMT tables.
func mpegTSCRC32(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		for i := 0; i < 8; i++ {
			if (crc^(uint32(b)<<24))&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
			b <<= 1
		}
	}
	return crc
}

// buildPATPacket returns a valid 188-byte MPEG-TS PAT packet declaring program 1
// at PMT PID patPMTKeepPMTPID. cc is the 4-bit continuity counter for PID 0.
//
// Packet layout:
//
//	pkt[0]     sync byte 0x47
//	pkt[1..2]  PUSI=1, PID=0x0000
//	pkt[3]     adaptation_field_control=0b01 (payload only), continuity_counter=cc
//	pkt[4]     pointer_field = 0x00
//	pkt[5..16] PAT section (table_id=0x00, section_length=13, transport_stream_id=1,
//	           version=0, current_next=1, program_number=1, PMT_PID=patPMTKeepPMTPID)
//	pkt[17..20] CRC-32 (big-endian)
//	pkt[21..187] 0xFF padding
func buildPATPacket(cc uint8) [188]byte {
	var pkt [188]byte
	pkt[0] = 0x47
	pkt[1] = 0x40 // PUSI=1, PID[12..8]=0
	pkt[2] = 0x00
	pkt[3] = 0x10 | (cc & 0x0F)
	pkt[4] = 0x00 // pointer_field
	s := pkt[5:]
	s[0] = 0x00 // table_id
	s[1] = 0xB0 // section_syntax=1, '0'=0, reserved=11, section_length[11..8]=0
	s[2] = 0x0D // section_length = 13
	s[3] = 0x00 // transport_stream_id high
	s[4] = 0x01 // transport_stream_id low
	s[5] = 0xC1 // reserved=11, version=00000, current_next=1
	s[6] = 0x00 // section_number
	s[7] = 0x00 // last_section_number
	s[8] = 0x00 // program_number high
	s[9] = 0x01 // program_number low
	s[10] = byte(0xE0 | ((patPMTKeepPMTPID >> 8) & 0x1F))
	s[11] = byte(patPMTKeepPMTPID & 0xFF)
	// CRC-32 over pkt[5..16] (12 bytes: table_id through PMT_PID)
	crc := mpegTSCRC32(pkt[5:17])
	s[12] = byte(crc >> 24)
	s[13] = byte(crc >> 16)
	s[14] = byte(crc >> 8)
	s[15] = byte(crc)
	for i := 21; i < 188; i++ {
		pkt[i] = 0xFF
	}
	return pkt
}

// buildPMTPacket returns a valid 188-byte MPEG-TS PMT packet for program 1,
// declaring H264 video (stream_type 0x1B, PID patPMTKeepVideoPID) and
// AAC audio (stream_type 0x0F, PID patPMTKeepAudioPID).
// cc is the 4-bit continuity counter for PID patPMTKeepPMTPID.
//
// Packet layout:
//
//	pkt[0]     sync byte 0x47
//	pkt[1..2]  PUSI=1, PID=patPMTKeepPMTPID (0x1000)
//	pkt[3]     adaptation_field_control=0b01, continuity_counter=cc
//	pkt[4]     pointer_field = 0x00
//	pkt[5..26] PMT section (table_id=0x02, section_length=23, program_number=1,
//	           PCR_PID=patPMTKeepVideoPID, video H264, audio AAC)
//	pkt[27..30] CRC-32 (big-endian)
//	pkt[31..187] 0xFF padding
func buildPMTPacket(cc uint8) [188]byte {
	var pkt [188]byte
	pkt[0] = 0x47
	pkt[1] = byte(0x40 | ((patPMTKeepPMTPID >> 8) & 0x1F))
	pkt[2] = byte(patPMTKeepPMTPID & 0xFF)
	pkt[3] = 0x10 | (cc & 0x0F)
	pkt[4] = 0x00 // pointer_field
	s := pkt[5:]
	s[0] = 0x02 // table_id (PMT)
	s[1] = 0xB0 // section_syntax=1, '0'=0, reserved=11, section_length[11..8]=0
	s[2] = 0x17 // section_length = 23
	s[3] = 0x00 // program_number high
	s[4] = 0x01 // program_number low
	s[5] = 0xC1 // reserved=11, version=00000, current_next=1
	s[6] = 0x00 // section_number
	s[7] = 0x00 // last_section_number
	// PCR_PID = video PID: reserved(3)=111 + pid(13)
	s[8] = byte(0xE0 | ((patPMTKeepVideoPID >> 8) & 0x1F))
	s[9] = byte(patPMTKeepVideoPID & 0xFF)
	// program_info_length = 0: reserved(4)=1111 + length(12)=0
	s[10] = 0xF0
	s[11] = 0x00
	// Video stream entry: stream_type=0x1B (H264), PID=patPMTKeepVideoPID, ES_info_length=0
	s[12] = 0x1B
	s[13] = byte(0xE0 | ((patPMTKeepVideoPID >> 8) & 0x1F))
	s[14] = byte(patPMTKeepVideoPID & 0xFF)
	s[15] = 0xF0
	s[16] = 0x00
	// Audio stream entry: stream_type=0x0F (AAC), PID=patPMTKeepAudioPID, ES_info_length=0
	s[17] = 0x0F
	s[18] = byte(0xE0 | ((patPMTKeepAudioPID >> 8) & 0x1F))
	s[19] = byte(patPMTKeepAudioPID & 0xFF)
	s[20] = 0xF0
	s[21] = 0x00
	// CRC-32 over pkt[5..26] (22 bytes: table_id through last ES_info_length)
	crc := mpegTSCRC32(pkt[5:27])
	s[22] = byte(crc >> 24)
	s[23] = byte(crc >> 16)
	s[24] = byte(crc >> 8)
	s[25] = byte(crc)
	for i := 31; i < 188; i++ {
		pkt[i] = 0xFF
	}
	return pkt
}

// startPATMPTKeepalive periodically sends PAT+PMT packets while the startup gate
// waits for ffmpeg to produce a valid IDR frame. By sending MPEG-TS program-structure
// information early, Plex's DASH packager can instantiate its consumer before the
// first real video frame arrives — directly preventing the "Failed to find consumer"
// startup race (dash_init_404, session opens but consumer never starts).
//
// The PAT declares program 1 at PMT PID patPMTKeepPMTPID; the PMT declares H264
// video (stream_type 0x1B) and AAC audio (stream_type 0x0F) at their respective PIDs.
// These PIDs match ffmpeg's mpegts muxer defaults so the keepalive packets are
// structurally continuous with both the bootstrap preamble and the real stream.
//
// Controlled by:
//
//	PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE=true     enable (default: false)
//	PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE_MS=500   interval in ms (default: 500)
//
// Returns a stop function (idempotent; blocks until the goroutine exits).
func startPATMPTKeepalive(
	ctx context.Context,
	dst io.Writer,
	flushBody func(),
	flusher http.Flusher,
	channelName, channelID, modeLabel string,
	start time.Time,
	interval time.Duration,
) func(string) {
	if dst == nil || interval <= 0 {
		return func(string) {}
	}
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	stopCh := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		reqField := gatewayReqIDField(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var sentBytes int64
		var ticks int
		var patCC, pmtCC uint8
		reason := "done"
		for {
			select {
			case <-ctx.Done():
				reason = "client-done"
				log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case reason = <-stopCh:
				log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case <-ticker.C:
			}
			pat := buildPATPacket(patCC)
			pmt := buildPMTPacket(pmtCC)
			patCC = (patCC + 1) & 0x0F
			pmtCC = (pmtCC + 1) & 0x0F
			n, err := dst.Write(pat[:])
			sentBytes += int64(n)
			if err != nil {
				reason = "write-error"
				log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive stop=%s err=%v bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, err, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			}
			n, err = dst.Write(pmt[:])
			sentBytes += int64(n)
			if err != nil {
				reason = "write-error"
				log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive stop=%s err=%v bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, err, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
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
