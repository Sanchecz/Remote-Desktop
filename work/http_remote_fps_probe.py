#!/usr/bin/env python3
"""Short-lived real-Agent FPS probe that only needs the Python standard library.

The script is designed to run on the RemoteIt VDS.  It creates an isolated
owner web session, starts one remote desktop session for an explicitly named
test device, moves one visible window in a closed loop, then restores the
window position.  Both temporary sessions are removed in ``finally``.
"""

from __future__ import annotations

import hashlib
import json
import os
import secrets
import statistics
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


BASE_URL = "https://supportgenesis.ru"
DEVICE_NAME = os.environ.get("REMOTEIT_TEST_DEVICE", "MyGenesis").strip() or "MyGenesis"
TARGET_FPS = int(os.environ.get("REMOTEIT_TEST_FPS", "30"))
TEST_SECONDS = float(os.environ.get("REMOTEIT_TEST_SECONDS", "12"))
MOTION_X = int(os.environ.get("REMOTEIT_MOTION_X", "720"))
MOTION_Y = int(os.environ.get("REMOTEIT_MOTION_Y", "345"))
AMPLITUDE_X = int(os.environ.get("REMOTEIT_MOTION_AMPLITUDE_X", "160"))
AMPLITUDE_Y = int(os.environ.get("REMOTEIT_MOTION_AMPLITUDE_Y", "36"))


def command(*args: str) -> str:
    completed = subprocess.run(args, check=True, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    return completed.stdout.strip()


release = command("readlink", "-f", "/opt/genesisit/current")
compose = [
    "sudo",
    "-n",
    "docker",
    "compose",
    "-f",
    release + "/compose.yaml",
    "-p",
    "genesisit",
    "--env-file",
    "/opt/genesisit/shared/.env",
]


def psql(query: str) -> str:
    return command(*compose, "exec", "-T", "db", "psql", "-U", "genesisit", "-d", "genesisit", "-Atqc", query)


def api(method: str, path: str, token: str, csrf: str, payload=None, timeout: float = 10):
    body = None if payload is None else json.dumps(payload, separators=(",", ":")).encode()
    request = urllib.request.Request(BASE_URL + path, data=body, method=method)
    request.add_header("Cookie", "genesis_session=" + token)
    request.add_header("X-CSRF-Token", csrf)
    if body is not None:
        request.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            data = response.read()
            return response.status, response.headers, data
    except urllib.error.HTTPError as exc:
        data = exc.read()
        raise RuntimeError(f"{method} {path}: HTTP {exc.code}: {data[:300]!r}") from exc


def json_api(method: str, path: str, token: str, csrf: str, payload=None):
    status, _, data = api(method, path, token, csrf, payload)
    return status, json.loads(data) if data else None


def pointer(token: str, csrf: str, session_id: str, events: list[dict]) -> None:
    api("POST", f"/api/desktop-sessions/{session_id}/input", token, csrf, {"events": events})


def main() -> None:
    if TARGET_FPS not in (0, 15, 30, 60):
        raise SystemExit("REMOTEIT_TEST_FPS must be Auto (0), 15, 30, or 60")
    token = secrets.token_urlsafe(48)
    csrf = secrets.token_urlsafe(32)
    session_row = ""
    desktop_id = ""
    stop = threading.Event()
    motion_error: list[str] = []
    try:
        owner_id = psql("SELECT id FROM users WHERE role='owner' ORDER BY created_at LIMIT 1")
        safe_device = DEVICE_NAME.replace("'", "''")
        device_id = psql(
            "SELECT id FROM devices WHERE name='" + safe_device + "' AND last_seen>now()-interval '90 seconds' "
            "ORDER BY last_seen DESC LIMIT 1"
        )
        if not owner_id or not device_id:
            raise RuntimeError("requested real test device is not online")
        token_hash = hashlib.sha256(token.encode()).hexdigest()
        csrf_hash = hashlib.sha256(csrf.encode()).hexdigest()
        session_row = psql(
            "INSERT INTO sessions(user_id,token_hash,csrf_hash,user_agent,expires_at) VALUES("
            f"'{owner_id}',decode('{token_hash}','hex'),decode('{csrf_hash}','hex'),'RemoteIt HTTP FPS probe',"
            "now()+interval '15 minutes') RETURNING id"
        )
        _, created = json_api(
            "POST",
            f"/api/devices/{device_id}/desktop-sessions",
            token,
            csrf,
            {"controlEnabled": False, "targetFps": TARGET_FPS, "cursorVisible": False},
        )
        desktop_id = created["id"]
        started = time.monotonic()
        motion_started_at = started + 3.0
        motion_ended_at = started + min(TEST_SECONDS - 2.0, 9.0)

        def motion() -> None:
            drag = False
            phase = 0
            next_send = motion_started_at
            try:
                while not stop.is_set() and time.monotonic() < motion_started_at:
                    stop.wait(0.02)
                if stop.is_set():
                    return
                json_api("PATCH", f"/api/desktop-sessions/{desktop_id}", token, csrf, {"controlEnabled": True})
                pointer(token, csrf, desktop_id, [
                    {"type": "key", "action": "down", "keyCode": 27},
                    {"type": "key", "action": "up", "keyCode": 27},
                    {"type": "pointer", "action": "move", "x": MOTION_X, "y": MOTION_Y},
                    {"type": "pointer", "action": "down", "button": "left", "x": MOTION_X, "y": MOTION_Y},
                ])
                drag = True
                period = 1.0 / max(30, TARGET_FPS)
                while not stop.is_set() and time.monotonic() < motion_ended_at:
                    now = time.monotonic()
                    if now < next_send:
                        stop.wait(next_send - now)
                        continue
                    phase += 1
                    cycle = phase % 120
                    if cycle < 30:
                        x = MOTION_X + cycle * AMPLITUDE_X // 30
                    elif cycle < 60:
                        x = MOTION_X + AMPLITUDE_X - (cycle - 30) * AMPLITUDE_X // 30
                    elif cycle < 90:
                        x = MOTION_X - (cycle - 60) * AMPLITUDE_X // 30
                    else:
                        x = MOTION_X - AMPLITUDE_X + (cycle - 90) * AMPLITUDE_X // 30
                    y_cycle = phase % 80
                    if y_cycle < 20:
                        y = MOTION_Y + y_cycle * AMPLITUDE_Y // 20
                    elif y_cycle < 40:
                        y = MOTION_Y + AMPLITUDE_Y - (y_cycle - 20) * AMPLITUDE_Y // 20
                    elif y_cycle < 60:
                        y = MOTION_Y - (y_cycle - 40) * AMPLITUDE_Y // 20
                    else:
                        y = MOTION_Y - AMPLITUDE_Y + (y_cycle - 60) * AMPLITUDE_Y // 20
                    pointer(token, csrf, desktop_id, [{"type": "pointer", "action": "move", "x": x, "y": y}])
                    next_send += period
                    if next_send < now - period:
                        next_send = now + period
            except Exception as exc:  # reported after deterministic cleanup
                motion_error.append(str(exc))
            finally:
                if drag:
                    try:
                        pointer(token, csrf, desktop_id, [
                            {"type": "pointer", "action": "move", "x": MOTION_X, "y": MOTION_Y},
                            {"type": "pointer", "action": "up", "button": "left", "x": MOTION_X, "y": MOTION_Y},
                        ])
                    except Exception as exc:
                        motion_error.append("release: " + str(exc))

        mover = threading.Thread(target=motion, daemon=True)
        mover.start()
        frames: list[tuple[float, str, int]] = []
        sequence_samples: list[tuple[float, int, int]] = []
        last_at = ""
        next_status = started
        poll_period = 1.0 / max(90, TARGET_FPS * 2)
        next_poll = started
        while time.monotonic() - started < TEST_SECONDS:
            now = time.monotonic()
            if now >= next_status:
                _, status = json_api("GET", f"/api/desktop-sessions/{desktop_id}", token, csrf)
                sequence_samples.append((now, int(status.get("frameSequence") or 0), int(status.get("producerFrameSequence") or 0)))
                next_status = now + 0.25
            if now < next_poll:
                time.sleep(min(0.005, next_poll - now))
                continue
            query = "?after=" + urllib.parse.quote(last_at) if last_at else ""
            code, headers, data = api("GET", f"/api/desktop-sessions/{desktop_id}/frame" + query, token, csrf)
            if code == 200 and data:
                frame_at = headers.get("X-RemoteIt-Frame-At", "")
                if frame_at and frame_at != last_at:
                    last_at = frame_at
                    frames.append((time.monotonic(), hashlib.sha256(data).hexdigest(), len(data)))
            next_poll += poll_period
            if next_poll < now - poll_period:
                next_poll = now + poll_period
        stop.set()
        mover.join(timeout=3)
        _, final_status = json_api("GET", f"/api/desktop-sessions/{desktop_id}", token, csrf)
        motion_frames = [item for item in frames if motion_started_at <= item[0] <= motion_ended_at]
        intervals = [(motion_frames[i][0] - motion_frames[i - 1][0]) * 1000 for i in range(1, len(motion_frames))]
        start_sample = min(sequence_samples, key=lambda item: abs(item[0] - motion_started_at))
        end_sample = min(sequence_samples, key=lambda item: abs(item[0] - motion_ended_at))
        elapsed = max(0.001, end_sample[0] - start_sample[0])
        result = {
            "device": DEVICE_NAME,
            "target_fps": TARGET_FPS,
            "agent_connected": final_status.get("agentConnected"),
            "frame": [final_status.get("frameWidth"), final_status.get("frameHeight")],
            "unique_frames": len({item[1] for item in frames}),
            "motion_frames": len(motion_frames),
            "viewer_motion_fps": round((len(motion_frames) - 1) / max(0.001, motion_frames[-1][0] - motion_frames[0][0]), 2) if len(motion_frames) > 1 else 0,
            "producer_motion_fps": round((end_sample[2] - start_sample[2]) / elapsed, 2),
            "motion_median_bytes": round(statistics.median(item[2] for item in motion_frames)) if motion_frames else 0,
            "motion_p95_ms": round(sorted(intervals)[max(0, round(len(intervals) * 0.95) - 1)], 1) if intervals else 0,
            "capture_diagnostics": final_status.get("captureDiagnostics"),
            "agent_error": final_status.get("agentError"),
            "motion_error": motion_error,
        }
        print(json.dumps(result, ensure_ascii=False, sort_keys=True))
        if motion_error or not result["agent_connected"] or result["motion_frames"] < 2:
            raise RuntimeError("real-Agent FPS probe did not produce a healthy motion stream")
    finally:
        stop.set()
        if desktop_id:
            try:
                api("DELETE", f"/api/desktop-sessions/{desktop_id}", token, csrf)
            except Exception:
                psql(
                    "UPDATE remote_desktop_sessions SET status='ended',frame=NULL,ended_at=now(),expires_at=now() "
                    "WHERE id='" + desktop_id.replace("'", "''") + "'"
                )
        if session_row:
            psql("DELETE FROM sessions WHERE id='" + session_row.replace("'", "''") + "'")


if __name__ == "__main__":
    main()
