// Package sound synthesises the short interaction cues — no audio assets are
// shipped. It is a tiny SteamOS-ish click vocabulary: soft tones with fast
// envelopes, played asynchronously through winmm.
package sound

import (
	"encoding/binary"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

var (
	winmm      = syscall.NewLazyDLL("winmm.dll")
	pPlaySound = winmm.NewProc("PlaySoundW")

	mu      sync.Mutex
	enabled = true
	volume  = 0.55
	buffers = map[string][]byte{}
	// keepAlive pins buffers while SND_ASYNC is still reading them; replacing
	// the map on a volume change must not free the buffer mid-playback.
	keepAlive [][]byte
)

const (
	sndAsync     = 0x0001
	sndNoDefault = 0x0002
	sndMemory    = 0x0004
)

// Init builds the cue buffers for the given volume (0..100).
func Init(vol int) {
	mu.Lock()
	defer mu.Unlock()
	setVolumeLocked(vol)
}

// SetEnabled turns cues on or off.
func SetEnabled(on bool) {
	mu.Lock()
	enabled = on
	mu.Unlock()
}

// SetVolume rebuilds the buffers at a new 0..100 volume.
func SetVolume(vol int) {
	mu.Lock()
	setVolumeLocked(vol)
	mu.Unlock()
}

func setVolumeLocked(vol int) {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	volume = float64(vol) / 100
	buffers = map[string][]byte{
		"startup": wav(seq(tone(392, 0.12, 0.12), tone(523.25, 0.14, 0.11), tone(659.25, 0.30, 0.10))),
		"launch":  wav(seq(tone(160, 0.10, 0.22), tone(196, 0.10, 0.20), tone(262, 0.22, 0.17))),
		"move":    wav(tone(2400, 0.026, 0.10)),
		"press":   wav(mix(tone(900, 0.035, 0.24), noise(0.012, 0.08, 9000))),
		"confirm": wav(seq(tone(880, 0.045, 0.22), tone(1320, 0.075, 0.18))),
		"back":    wav(seq(tone(720, 0.040, 0.22), tone(520, 0.070, 0.20))),
		"error":   wav(seq(tone(220, 0.070, 0.30), tone(180, 0.080, 0.24))),
		"toggle":  wav(seq(tone(1040, 0.030, 0.20), tone(1560, 0.030, 0.16))),
		"tray":    wav(seq(tone(740, 0.045, 0.14), tone(494, 0.080, 0.12))),
	}
}

// Play emits one cue asynchronously; unknown kinds are ignored.
func Play(kind string) {
	mu.Lock()
	on := enabled
	buf := buffers[kind]
	if on && len(buf) > 0 {
		keepAlive = append(keepAlive, buf)
		if len(keepAlive) > 32 {
			keepAlive = keepAlive[1:]
		}
	}
	mu.Unlock()
	if !on || len(buf) == 0 {
		return
	}
	pPlaySound.Call(uintptr(unsafe.Pointer(&buf[0])), 0, sndAsync|sndNoDefault|sndMemory)
}

func tone(freq, dur, amp float64) []float64 {
	n := int(44100 * dur)
	out := make([]float64, n)
	for i := range out {
		t := float64(i) / 44100
		env := math.Exp(-t / (dur * 0.35))
		out[i] = math.Sin(2*math.Pi*freq*t) * env * amp * volume
	}
	return out
}

func noise(dur, amp, hz float64) []float64 {
	n := int(44100 * dur)
	out := make([]float64, n)
	seed := uint32(2463534242)
	for i := range out {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		t := float64(i) / 44100
		env := math.Exp(-t / (dur * 0.5))
		out[i] = (float64(seed%1000)/500 - 1) * env * amp * volume
	}
	return out
}

func seq(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func mix(a, b []float64) []float64 {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		if i < len(a) {
			out[i] += a[i]
		}
		if i < len(b) {
			out[i] += b[i]
		}
	}
	return out
}

func wav(samples []float64) []byte {
	const hdr = 44
	buf := make([]byte, hdr+len(samples)*2)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(buf)-8))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], 44100)
	binary.LittleEndian.PutUint32(buf[28:], 44100*2)
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(len(samples)*2))
	for i, s := range samples {
		if s > 1 {
			s = 1
		}
		if s < -1 {
			s = -1
		}
		binary.LittleEndian.PutUint16(buf[hdr+i*2:], uint16(int16(s*32767)))
	}
	return buf
}
