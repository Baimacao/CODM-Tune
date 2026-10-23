package sound

import "testing"

func TestCueBuffers(t *testing.T) {
	Init(55)
	for _, kind := range []string{"move", "press", "confirm", "back", "error", "toggle"} {
		b := buffers[kind]
		if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
			t.Fatalf("%s is not a WAV buffer (len=%d)", kind, len(b))
		}
	}
}
