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

The bridge exposes `GET/POST /api/cameras/<cam>/floodlight` (`{"on":bool}`).
In Scrypted, create one script per floodlight (Scripts → Add Script):

```ts
const url = 'http://127.0.0.1:5080/api/cameras/backyard_cam/floodlight';
class Floodlight implements OnOff {
constructor() { this.refresh(); }
async refresh() { try { const r = await fetch(url); device.on = (await r.json()).on; } catch (e) { console.error('floodlight state', e); } }
async set(on: boolean) { const r = await fetch(url, { method: 'POST', body: JSON.stringify({ on }) }); if (!r.ok) throw new Error(await r.text()); device.on = on; }
turnOn() { return this.set(true); }
turnOff() { return this.set(false); }
}
device.handleTypes(ScryptedInterface.OnOff);
export default Floodlight;
```

Save and run it, set the device **Type** to Light, and enable the
**HomeKit** extension. Lights are bridged accessories: pair the "Scrypted"
bridge once (QR on the HomeKit plugin page) and every light added later
appears in Home automatically.
