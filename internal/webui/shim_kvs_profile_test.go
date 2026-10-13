package webui

import (
	"testing"

	"github.com/IDisposable/docker-wyze-bridge/internal/wyzeapi"
)

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
		hd := wyzeapi.CameraInfo{Model: tt.model}.WebRTCHDProfile()
		if got := kvsVideoProfile(hd, tt.quality); got != tt.want {
			t.Errorf("kvsVideoProfile(%q, %q) = %q, want %q", tt.model, tt.quality, got, tt.want)
		}
	}
}
