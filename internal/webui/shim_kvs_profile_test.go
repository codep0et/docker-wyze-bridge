package webui

import "testing"

func TestKVSVideoProfile(t *testing.T) {
	tests := []struct {
		model, quality, want string
	}{
		{"HL_CFL2", "hd", "2k"},
		{"HL_CFL2", "", "2k"},
		{"HL_CFL2", "sd", ""},
		{"HL_CFL2", "SD30", ""},
		{"GW_DBD", "hd", ""},
		{"HL_CAM4", "hd", ""},
	}
	for _, tt := range tests {
		if got := kvsVideoProfile(tt.model, tt.quality); got != tt.want {
			t.Errorf("kvsVideoProfile(%q, %q) = %q, want %q", tt.model, tt.quality, got, tt.want)
		}
	}
}
