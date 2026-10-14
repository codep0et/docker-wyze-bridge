package webui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/IDisposable/docker-wyze-bridge/internal/wyzeapi"
)

// Integrated-floodlight control commands (HL_CFL2). The Wyze cloud
// mirrors the light's state in property P1056 but ignores writes to it,
// so the switch has to go to the camera itself.
const (
	cmdGetFloodlightInfo uint16 = 10788
	cmdSetFloodlightInfo uint16 = 10790
)

// floodlightMaxBrightness is the camera's full-brightness value; the
// API speaks percent like HomeKit does.
const floodlightMaxBrightness = 254

// floodlightInfo is the envelope both commands use. Values are strings:
// switch / motionSwitch "1" = on, "2" = off; result "1" = accepted.
// motionSwitch is the light's own motion activation (the camera keeps
// its duration and schedule).
type floodlightInfo struct {
	FloodlightInfo struct {
		ProtocolVersion string `json:"protocolVersion,omitempty"`
		Switch          string `json:"switch,omitempty"`
		Brightness      string `json:"brightness,omitempty"`
		MotionSwitch    string `json:"motionSwitch,omitempty"`
		Result          string `json:"result,omitempty"`
	} `json:"floodlightInfo"`
}

func onOff(v bool) string {
	if v {
		return "1"
	}
	return "2"
}

func brightnessToPercent(raw string) (int, bool) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return int(math.Round(float64(n) * 100 / floodlightMaxBrightness)), true
}

func percentToBrightness(pct int) string {
	return strconv.Itoa(int(math.Round(float64(pct) * floodlightMaxBrightness / 100)))
}

// handleFloodlight serves /api/cameras/{name}/floodlight.
// GET returns {"on":bool,"brightness":0-100,"motion":bool}; POST takes
// any of those fields and changes only the ones given.
func (s *Server) handleFloodlight(w http.ResponseWriter, r *http.Request, info wyzeapi.CameraInfo) {
	if !info.HasFloodlight() {
		writeJSONError(w, http.StatusBadRequest, "camera has no floodlight")
		return
	}
	api := s.go2rtc()
	if api == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "go2rtc not ready")
		return
	}
	// The command session is independent of the video path, so always
	// build a wyze:// URL even when the camera streams over WebRTC.
	camURL := info.StreamURL("sd")

	switch r.Method {
	case http.MethodGet:
		raw, err := api.WyzeCommand(r.Context(), camURL, cmdGetFloodlightInfo, "")
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		var resp floodlightInfo
		if err := json.Unmarshal([]byte(raw), &resp); err != nil || resp.FloodlightInfo.Switch == "" {
			writeJSONError(w, http.StatusBadGateway, "camera did not report floodlight state")
			return
		}
		out := map[string]interface{}{
			"on":     resp.FloodlightInfo.Switch == "1",
			"motion": resp.FloodlightInfo.MotionSwitch == "1",
		}
		if pct, ok := brightnessToPercent(resp.FloodlightInfo.Brightness); ok {
			out["brightness"] = pct
		}
		writeJSON(w, out)

	case http.MethodPost:
		var body struct {
			On         *bool `json:"on"`
			Brightness *int  `json:"brightness"`
			Motion     *bool `json:"motion"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
			(body.On == nil && body.Brightness == nil && body.Motion == nil) {
			writeJSONError(w, http.StatusBadRequest, `body must set at least one of "on", "brightness" (1-100), "motion"`)
			return
		}
		if body.Brightness != nil && (*body.Brightness < 1 || *body.Brightness > 100) {
			writeJSONError(w, http.StatusBadRequest, "brightness must be 1-100")
			return
		}
		// protocolVersion is mandatory: without it the camera answers
		// result "2" and leaves the light as it was.
		var cmd floodlightInfo
		cmd.FloodlightInfo.ProtocolVersion = "1"
		out := map[string]interface{}{"status": "ok"}
		if body.On != nil {
			cmd.FloodlightInfo.Switch = onOff(*body.On)
			out["on"] = *body.On
		}
		if body.Brightness != nil {
			cmd.FloodlightInfo.Brightness = percentToBrightness(*body.Brightness)
			out["brightness"] = *body.Brightness
		}
		if body.Motion != nil {
			cmd.FloodlightInfo.MotionSwitch = onOff(*body.Motion)
			out["motion"] = *body.Motion
		}
		payload, _ := json.Marshal(cmd)

		raw, err := api.WyzeCommand(r.Context(), camURL, cmdSetFloodlightInfo, string(payload))
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		var resp floodlightInfo
		if err := json.Unmarshal([]byte(raw), &resp); err != nil || resp.FloodlightInfo.Result != "1" {
			writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("camera rejected floodlight command: %s", raw))
			return
		}
		writeJSON(w, out)

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}
