package tune

import "testing"

// The shipped FOV payloads are 8 bytes and decode as doubles 1.0/1.15; the
// name split must keep view and weapon keys apart.
func TestSplitFOV(t *testing.T) {
	view, weapon := splitFOV([]string{
		"CODM_MPMainCameraFovInGame_h1",
		"CODM_MPMainCameraFovInGame_3p_h2",
		"CODM_MPWeaponFov_h3",
		"CODM_DMZADSFovCurve_h4",
	})
	if len(view) != 2 || len(weapon) != 1 {
		t.Fatalf("view=%v weapon=%v", view, weapon)
	}
	if view[0] != "CODM_MPMainCameraFovInGame_h1" || weapon[0] != "CODM_MPWeaponFov_h3" {
		t.Fatalf("unexpected split: %v %v", view, weapon)
	}
}

func TestEstimateLossBounds(t *testing.T) {
	cases := []OverlayStats{
		{},
		{RTT: 20, Jitter: 1},
		{RTT: 300, Jitter: 90, Stutter: 40},
	}
	for _, c := range cases {
		got := estimateLoss(c)
		if got < 0 || got > 35 {
			t.Fatalf("estimateLoss(%+v) = %v, outside 0..35", c, got)
		}
	}
}
