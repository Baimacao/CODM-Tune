package crosshair

import "testing"

func TestParseConsoleAndValorant(t *testing.T) {
	s, err := Parse("cl_crosshairsize 7; cl_crosshairthickness 2; cl_crosshairgap -2; cl_crosshairdot 1; cl_crosshaircolor_r 255; cl_crosshaircolor_g 0; cl_crosshaircolor_b 0")
	if err != nil {
		t.Fatal(err)
	}
	if s.Length != 7 || s.Gap != 0 || s.Color != "#FF0000" || !s.Dot {
		t.Fatalf("console parse: %+v", s)
	}
	v, err := Parse("0;P;c;5;u;00FF00;o;1;d;1;0t;2;0l;4;0o;2;0a;8;1b;0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Color != "#00FF00" || v.Thickness != 2 || v.Length != 8 || v.Gap != 2 {
		t.Fatalf("valorant parse: %+v", v)
	}
}

func TestParseJSON(t *testing.T) {
	s, err := Parse(`{"style":"dot","length":3,"thickness":2,"gap":4,"color":"#FFFFFF","alpha":180,"dot":true}`)
	if err != nil || s.Style != "dot" || s.Alpha != 180 {
		t.Fatalf("json parse: %+v %v", s, err)
	}
}
