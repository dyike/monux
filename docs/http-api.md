# HTTP API

`monux serve` exposes the configured monitor switcher as JSON over HTTP. It is
an optional entry point in the same binary, not a separate project or a second
DDC implementation.

## Endpoints

### `GET /healthz`

Checks that the HTTP process is accepting requests. It does not access the
monitor and does not require authentication.

```json
{"status":"ok"}
```

### `GET /api/v1/status`

Reads VCP `0x60` from the configured monitor.

```json
{"name":"mac","input":"0x11","value":17,"connector":"HDMI 1"}
```

`name` is omitted when the current value has no matching configured name.

### `GET /api/v1/inputs`

Lists the named inputs loaded from the configuration, sorted by name.

```json
{
  "inputs": [
    {"name":"linux","input":"0x0f","value":15,"connector":"DisplayPort 1"},
    {"name":"mac","input":"0x11","value":17,"connector":"HDMI 1"}
  ]
}
```

This endpoint reports configured names. Use the CLI command `monux inputs` to
perform the slower DDC/CI capabilities discovery against the monitor.

### `GET /api/v1/capabilities`

Performs native DDC/CI capabilities discovery, reads the current input, and
merges both with configured names:

```json
{
  "inputs": [
    {
      "input":"0x0f",
      "value":15,
      "connector":"DisplayPort 1",
      "names":["linux"],
      "reported":true,
      "current":false
    },
    {
      "input":"0x11",
      "value":17,
      "connector":"HDMI 1",
      "names":["mac"],
      "reported":true,
      "current":true
    }
  ]
}
```

If either hardware query fails, configured values are still returned and the
corresponding `reported` or `current` fields are `null`; the response also
includes a `warnings` string array. This endpoint can take several
seconds on a slow DDC/CI transport; UI clients should use it for discovery and
poll the lighter `/api/v1/status` endpoint for routine updates.

### `POST /api/v1/switch/{name}`

Sets VCP `0x60` to the value configured for `name`.

```json
{"name":"linux","input":"0x0f","value":15,"connector":"DisplayPort 1"}
```

An unknown name returns HTTP `404`. A native monitor transport failure returns
HTTP `500`, and all error responses use this shape:

```json
{"error":"error description"}
```

### `POST /api/v1/set/{value}`

Sets a raw VCP `0x60` value using decimal or `0x`-prefixed hexadecimal input:

```bash
curl -X POST http://127.0.0.1:8765/api/v1/set/0x0f
```

An invalid value returns HTTP `400`. Prefer named switching in normal clients;
the raw endpoint is intended for setup and diagnostics.

## Authentication

Set `MONUX_HTTP_TOKEN` or pass `--token` to require this header on every
`/api/v1/*` request:

```http
Authorization: Bearer your-token
```

An invalid or missing token returns HTTP `401`. Prefer the environment variable
because a command-line token may be visible in the operating system's process
list.

Authentication is disabled when the token is empty. `monux` prints a warning
if an unauthenticated server is explicitly bound beyond a loopback address.
Bearer tokens sent over plain HTTP are not encrypted; use this mode only on a
trusted LAN, or place `monux` behind a TLS reverse proxy when the network is not
trusted.

## Operating model

The server loads configuration once at startup. Restart it after editing input
names or `monitor.id`. Monitor operations are serialized because many DDC/CI
transports do not tolerate overlapping requests.

The server controls the monitor locally through the video connection of the
machine where it runs. It does not use SSH and does not forward DDC commands to
another computer. Whether a monitor accepts DDC commands from an inactive input
remains monitor-dependent.
