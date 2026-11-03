#!/usr/bin/env python3
"""End-to-end check of the built Windows app: it starts, a second launch
hands over to it, and it quits when asked.

    python -I scripts/app_smoke.py <path to health-trail.exe>

Runs `health-trail.exe app` on a throwaway data dir and port. Quitting goes
through `taskkill` without /F: it sends WM_CLOSE to the app's windows, and
Wails quits on that the same way as the tray's Quit and an update's restart
(which waits for the old process to be gone). Exits non-zero on the first
failure. Windows only.
"""
import os, socket, subprocess, sys, tempfile, time, urllib.request

binary = sys.argv[1]
data = tempfile.mkdtemp(prefix="health-trail-app-smoke-")
with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
base = f"http://127.0.0.1:{port}"
args = [binary, "app", "-data", data, "-port", str(port)]
app = subprocess.Popen(args)


def fail(msg):
    print("FAIL:", msg)
    subprocess.run(["taskkill", "/F", "/T", "/PID", str(app.pid)], capture_output=True)
    sys.exit(1)


def get(path):
    with urllib.request.urlopen(base + path, timeout=5) as r:
        return r.status, r.headers.get("Content-Type", ""), r.read()


def wait(check, seconds, what):
    end = time.time() + seconds
    while time.time() < end:
        if check():
            return
        time.sleep(0.5)
    fail(what)


def answers():
    if app.poll() is not None:
        fail(f"the app exited with {app.returncode} while starting")
    try:
        return get("/api/info")[0] == 200
    except OSError:
        return False


# it starts: the window's process serves the API and the UI
wait(answers, 60, f"the app didn't answer on {base} within 60s")
print("ok  started")
for path, want in (("/", "text/html"), ("/icon.svg", "image/svg+xml")):
    status, ctype, _ = get(path)
    # the type the UI is served with must not come from the Windows registry
    # (a broken sidebar icon in 1.8.4)
    if status != 200 or not ctype.startswith(want):
        fail(f"{path}: {status} {ctype!r}, want {want}")
    print(f"ok  {path} {ctype}")

# a second launch on the same data hands over to the running one and exits
second = subprocess.Popen(args)
try:
    second.wait(30)
except subprocess.TimeoutExpired:
    second.kill()
    fail("a second launch on the same data didn't exit")
if app.poll() is not None or not answers():
    fail("the first app stopped after a second launch")
print("ok  second launch handed over")

# quitting: the process is gone, the port free
subprocess.run(["taskkill", "/PID", str(app.pid)], capture_output=True)
try:
    app.wait(30)
except subprocess.TimeoutExpired:
    fail("the app didn't quit within 30s of WM_CLOSE (an update's restart would wait forever)")
print(f"ok  quit ({app.returncode})")
with socket.socket() as s:
    try:
        s.bind(("127.0.0.1", port))
    except OSError:
        fail(f"port {port} still taken after quitting")
print("ok  port free")
