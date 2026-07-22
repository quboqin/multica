# WebView Semantic Testing

## Decision

Multica models hybrid mobile testing as one semantic case executed by platform
adapters. Product pages implemented in H5 are observed and controlled through
the browser document. Native adapters are used only when a case crosses into
platform UI such as permissions, camera capture, file pickers, biometrics,
notifications, or another native screen.

Semantic cases must not encode screen coordinates. Coordinates remain an escape
hatch for media-only surfaces that expose no structured accessibility or
document information.

## Runtime topology

```text
Issue test case
      |
      v
Semantic runner
      |
      +-- Web adapter: DOM, ARIA, route, console, network
      +-- Android adapter: WebView CDP + ADB/UIAutomator
      +-- iOS adapter: WebKit inspector + XCTest/WDA
      +-- Browser adapter: Playwright/CDP
```

All adapters publish the same vocabulary:

- `snapshot`: URL, title, visible semantic elements, and native context;
- `fill`: set a field and emit user-equivalent input/change events;
- `tap`: activate an element by semantic selector;
- `assert`: verify visibility, value, text, route, or native state;
- `wait_for`: wait for an assertion while the application settles;
- `native`: use a platform capability such as a permission or camera fixture.

## Selector contract

Selectors prefer stable product meaning over implementation position:

```json
{
  "role": "textbox",
  "name": "OTP",
  "test_id": "login-otp",
  "css": ""
}
```

Resolution order is `test_id`, accessible role/name, associated label,
placeholder, visible text, then explicit CSS. Zero or multiple matches fail with
evidence; the runner never guesses a coordinate.

Product repositories should add `data-testid`, labels, and ARIA names to
business-critical controls. Existing pages remain observable without them, but
stable identifiers make cases resilient to copy and layout changes.

## H5 development loop

The application shell must not load a deployed production H5 during a preview
task. The agent starts the H5 development server for the task checkout, then
launches the stage/debug shell with a task-scoped URL:

```text
H5 checkout + HMR
      |
      +-- Android emulator: http://10.0.2.2:<port>
      +-- Android USB device: adb reverse + http://127.0.0.1:<port>
      +-- iOS simulator: http://127.0.0.1:<port>
      +-- iOS USB device: host tunnel or managed HTTPS preview URL
```

Only preview builds accept a launch URL and enable WebView inspection. Release
builds use their configured production URL and do not expose debugging.

## Case example

```json
{
  "name": "OTP login",
  "steps": [
    {"action": "wait_for", "selector": {"role": "textbox", "name": "OTP"}},
    {"action": "fill", "selector": {"role": "textbox", "name": "OTP"}, "value": "123456"},
    {"action": "assert", "selector": {"role": "textbox", "name": "OTP"}, "value": "123456"},
    {"action": "tap", "selector": {"role": "button", "name": "Sah"}}
  ]
}
```

The Issue viewer shows the real Android or iOS container over WebRTC while the
semantic runner controls the embedded document. Each step records its selector,
resolved element, result, duration, screenshot time, and console/network errors.

## Platform capability boundary

| Capability | H5 step | Native adapter |
| --- | --- | --- |
| Form input and buttons | DOM action | None |
| Camera page controls | DOM action | Inject fixture or operate camera UI |
| Runtime permission | Observe request | Grant/deny system dialog |
| File upload | Activate input | Attach managed fixture |
| Face/identity SDK | Invoke bridge | Test double or dedicated device flow |
| Push/deep link | Assert H5 result | Deliver platform event |

This boundary lets Android, iOS, and browser runs share business cases without
pretending that their platform capability implementations are identical.
