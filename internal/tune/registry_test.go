package tune

import "testing"

func TestClassifyRegValues(t *testing.T) {
	fps, fov := classifyRegValues([]string{
		"FramerateCustomizeValue_h123",
		"CODM_1_iMSDK_CN_MPMainCameraFovInGame_h1",
		"CODM_1_iMSDK_CN_BRMainCameraFovInGame_3p_h2",
		"FPSLevel_h998",
	})
	if fps != "FramerateCustomizeValue_h123" {
		t.Fatalf("fps key not found: %q", fps)
	}
	if len(fov) != 2 {
		t.Fatalf("expected 2 FOV keys, got %v", fov)
	}
	fps2, fov2 := classifyRegValues([]string{"CODM_ALL_EnableLobbyDynamicFrameRate_h1"})
	if fps2 != "" || len(fov2) != 0 {
		t.Fatalf("unrelated keys must not match: %q %v", fps2, fov2)
	}
}

func TestFindFPSKeys(t *testing.T) {
	v, e := findFPSKeys([]string{
		"CODM_1_iMSDK_CN_EnableFramerateCustomize_h1",
		"CODM_1_iMSDK_CN_FramerateCustomizeValue_h2",
		"CODM_1_iMSDK_CN_MPWeaponFov_h3",
	})
	if v == "" || e == "" {
		t.Fatalf("value=%q enable=%q", v, e)
	}
	if v != "CODM_1_iMSDK_CN_FramerateCustomizeValue_h2" {
		t.Fatalf("value key = %q", v)
	}
}
