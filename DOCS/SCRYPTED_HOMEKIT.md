# Scrypted → Apple Home setup

How this fork's bridge is wired into Apple Home through Scrypted. Scrypted
runs next to the bridge and reads its streams over loopback RTSP.

## Cameras

Add each camera with the **RTSP Camera** plugin:

| Camera | Stream 1 | Stream 2 |
|---|---|---|
| Floodlight V2 (`HL_CFL2`) | `rtsp://127.0.0.1:8554/<cam>_hd` (2K, on demand) | `rtsp://127.0.0.1:8554/<cam>` (360p) |
| Doorbell Duo (`GW_DBD`) | `rtsp://127.0.0.1:8554/<cam>` | — |

Settings per floodlight:

- **Streams:** Local and Remote stream = Stream 1; Low Resolution, Local
  Recording and Remote Recording = Stream 2. No prebuffered streams (keeps
  every camera on demand; several 2K streams at once overload the cameras'
  shared 2.4 GHz uplink).
- **Snapshot → Snapshot URL:** `http://127.0.0.1:1984/api/frame.jpeg?src=<cam>`.
  Without a prebuffer and without this URL, Scrypted has no snapshot source
  and Home tiles show "Snapshot unavailable".
- **HomeKit → Standalone Accessory Mode:** on. Each camera pairs with its own
  QR code.

Leave the battery doorbell without a Snapshot URL and without prebuffer:
every snapshot would wake it (~17 s cold start) and drain the battery.

In the Home app pick **Stream** (not Stream and Record) for every camera;
recording keeps the stream open for the hub's analysis.

## Remote live view

Home asks for a small stream when you watch from outside the home
(640x360 at ~132 kbps on cellular, stepping up to 1280x720 at ~299 kbps
when the link holds). Scrypted (without its paid NVR plugin) forwards the
chosen stream as-is and cannot adapt, so:

- the 2K stream (~1.4 Mbps) overwhelms the relay and freezes;
- the 360p stream (~175 kbps) works but looks soft on a phone in
  landscape;
- a 720p stream encoded for the purpose works best.

Make `<cam>_remote` with a go2rtc config file passed through
`GO2RTC_EXTRA_CONFIG`:

```yaml
ffmpeg:
  h264_remote: "-c:v libx264 -preset veryfast -tune zerolatency -x264-params sliced-threads=0 -profile:v main -b:v 280k -maxrate 300k -bufsize 600k -g 100 -keyint_min 100 -sc_threshold 0 -pix_fmt yuv420p"

streams:
  backyard_cam_remote: ffmpeg:backyard_cam_hd#video=h264_remote#width=1280#height=720#audio=copy
```

Then add `rtsp://127.0.0.1:8554/<cam>_remote` as the camera's third RTSP
URL in Scrypted and set **Remote (Medium Resolution) Stream** to Stream 3.
The encoder only runs while someone watches remotely (~40% of one core on
an Apple silicon Mac).

- `sliced-threads=0` is required: `-tune zerolatency` otherwise cuts each
  frame into ~10 slices and Home never displays the stream.
- x264 holds quality much better than `h264_videotoolbox` at ~300 kbps.
- A keyframe every 5 s (`-g 100` at ~20 fps) spends fewer bits on refreshes;
  start-up takes ~5 s.

## HomeKit plugin network settings (macOS)

Scrypted's default mDNS advertiser (ciao) asks `networksetup
-getairportnetwork` whether Wi-Fi is up; on macOS 26 that reports "not
associated" even when connected, so the cameras are announced on loopback
only and pairing fails with "Accessory not found". Set:

- HomeKit plugin → Network → **mDNS Advertiser:** `bonjour-hap`
- HomeKit plugin → Network → **mDNS Interfaces:** the LAN interface name
  typed as free text (e.g. `en1`)
- Scrypted Settings → **Server Addresses:** the host's LAN IP

Check with `dns-sd -L "<accessory name>" _hap._tcp` and `dns-sd -G v4v6
<host>`: the host should resolve to the LAN IPv4 address (plus IPv6), not
127.0.0.1 or virtual-machine bridge addresses. On Linux (ciao uses `ip
link`), the defaults are expected to work.

## Floodlight switches

The bridge exposes `GET/POST /api/cameras/<cam>/floodlight`:
`{"on":bool,"brightness":1-100,"motion":bool}`. A POST changes only the
fields it carries. `motion` is the light's own motion activation (the
camera keeps its duration and schedule). Setting `brightness` also turns
the light on, as the Wyze app's slider does.

The camera has a single brightness, shared with motion lighting: whatever
Home's slider last set is what the light uses when motion triggers it (the
Wyze app shows the same value). On/off alone never changes it. The camera
cannot store a new brightness without lighting up, so restoring it quietly
after a dimmed session is not possible. Create Home scenes with the lights at
100%, or a scene will save and reapply its dimmed level.

In Scrypted, create one script per floodlight (Scripts → Add Script):

```ts
const url = 'http://127.0.0.1:5080/api/cameras/backyard_cam/floodlight';
class Floodlight implements OnOff, Brightness {
constructor() { this.refresh(); }
async refresh() { try { const s = await (await fetch(url)).json(); device.on = s.on; device.brightness = s.brightness; } catch (e) { console.error('floodlight state', e); } }
async send(body: any) { const r = await fetch(url, { method: 'POST', body: JSON.stringify(body) }); if (!r.ok) throw new Error(await r.text()); }
async turnOn() { await this.send({ on: true }); device.on = true; }
async turnOff() { await this.send({ on: false }); device.on = false; }
async setBrightness(b: number) { await this.send({ brightness: Math.max(1, Math.round(b)) }); device.brightness = b; device.on = true; }
}
device.handleTypes(ScryptedInterface.OnOff, ScryptedInterface.Brightness);
export default Floodlight;
```

Save and run it, set the device **Type** to Light, and enable the
**HomeKit** extension. Lights are bridged accessories: pair the "Scrypted"
bridge once (QR on the HomeKit plugin page) and every light added later
appears in Home automatically.

For a motion-activation switch per floodlight, add another script with
**Type** Switch:

```ts
const url = 'http://127.0.0.1:5080/api/cameras/backyard_cam/floodlight';
class MotionLight implements OnOff {
constructor() { this.refresh(); }
async refresh() { try { device.on = (await (await fetch(url)).json()).motion; } catch (e) { console.error('motion state', e); } }
async set(on: boolean) { const r = await fetch(url, { method: 'POST', body: JSON.stringify({ motion: on }) }); if (!r.ok) throw new Error(await r.text()); device.on = on; }
turnOn() { return this.set(true); }
turnOff() { return this.set(false); }
}
device.handleTypes(ScryptedInterface.OnOff);
export default MotionLight;
```
