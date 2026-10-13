package camera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/IDisposable/docker-wyze-bridge/internal/config"
	"github.com/IDisposable/docker-wyze-bridge/internal/go2rtcmgr"
	"github.com/IDisposable/docker-wyze-bridge/internal/wyzeapi"
)

// A WebRTC camera with a known HD profile gets an on-demand "<cam>_hd"
// stream next to its main one; everything else gets only the main one.

func TestManager_HDStream_WebRTCCameraWithProfile(t *testing.T) {
	mgr, go2rtcAPI := newTestManager(t)

	cam := NewCamera(wyzeapi.CameraInfo{
		Name:  "backyard_cam",
		MAC:   "AABBCCDDEE10",
		Model: "HL_CFL2",
		LanIP: "10.0.0.8",
	}, "sd", true, false)
	cam.SetForceWebRTC(true)
	mgr.cameras["backyard_cam"] = cam

	if url := mgr.hdStreamSourceFor(cam); !strings.Contains(url, "/internal/wyze/webrtc/backyard_cam_hd#format=wyze") {
		t.Errorf("hd source = %q, want the _hd shim URL", url)
	}

	mgr.connectCamera(context.Background(), cam)

	streams, _ := go2rtcAPI.ListStreams(context.Background())
	for _, name := range []string{"backyard_cam", "backyard_cam_hd"} {
		if _, has := streams[name]; !has {
			t.Errorf("stream %q should be registered", name)
		}
	}

	mgr.StopStream(context.Background(), "backyard_cam")
	streams, _ = go2rtcAPI.ListStreams(context.Background())
	for _, name := range []string{"backyard_cam", "backyard_cam_hd"} {
		if _, has := streams[name]; has {
			t.Errorf("stream %q should be removed on stop", name)
		}
	}
}

func TestManager_HDStream_NotForOtherCameras(t *testing.T) {
	mgr, go2rtcAPI := newTestManager(t)

	tutk := NewCamera(wyzeapi.CameraInfo{Name: "tutk_fl", MAC: "AABBCCDDEE11", Model: "HL_CFL2", LanIP: "10.0.0.9"}, "hd", true, false)
	doorbell := NewCamera(wyzeapi.CameraInfo{Name: "front_door", MAC: "AABBCCDDEE12", Model: "GW_DBD"}, "hd", true, false)
	for _, cam := range []*Camera{tutk, doorbell} {
		mgr.cameras[cam.Name()] = cam
		if url := mgr.hdStreamSourceFor(cam); url != "" {
			t.Errorf("%s: hd source = %q, want none", cam.Name(), url)
		}
		mgr.connectCamera(context.Background(), cam)
	}

	streams, _ := go2rtcAPI.ListStreams(context.Background())
	for name := range streams {
		if strings.HasSuffix(name, HDStreamSuffix) {
			t.Errorf("unexpected HD stream %q", name)
		}
	}
}

func TestManager_SetQuality_KeepsWebRTCSource(t *testing.T) {
	// Record the source go2rtc is given on each PUT /api/streams.
	var mu sync.Mutex
	sources := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" && r.Method == http.MethodPut {
			mu.Lock()
			sources[r.URL.Query().Get("name")] = r.URL.Query().Get("src")
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := &config.Config{Quality: "hd", Audio: true, CamOverrides: make(map[string]config.CamOverride), BridgePort: 5080}
	wapi := wyzeapi.NewClient(wyzeapi.Credentials{}, "test", zerolog.Nop())
	mgr := NewManager(cfg, wapi, go2rtcmgr.NewAPIClient(srv.URL, zerolog.Nop()), zerolog.Nop())

	cam := NewCamera(wyzeapi.CameraInfo{Name: "backyard_cam", MAC: "AABBCCDDEE13", Model: "HL_CFL2", LanIP: "10.0.0.8"}, "hd", true, false)
	cam.SetForceWebRTC(true)
	mgr.cameras["backyard_cam"] = cam

	if err := mgr.SetQuality(context.Background(), "backyard_cam", "sd"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if src := sources["backyard_cam"]; !strings.HasPrefix(src, "webrtc:") {
		t.Errorf("SetQuality re-added a WebRTC camera with source scheme %q, want webrtc:", strings.SplitN(src, ":", 2)[0])
	}
}
