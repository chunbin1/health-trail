#!/usr/bin/env python3
"""End-to-end check of the built Windows app: it starts, a second launch
hands over to it, and it quits when asked.

    python -I scripts/app_smoke.py <path to health-trail.exe>

Runs `health-trail.exe app` on a throwaway data dir and port. Quitting sends
WM_CLOSE to Wails's hidden main-thread window, which quits the app the same
way as the tray's Quit and an update's restart (which waits for the old
process to be gone); WM_CLOSE to the app's window only hides it. Exits
non-zero on the first failure. Windows only.
"""
import ctypes, os, socket, subprocess, sys, tempfile, time, urllib.request
from ctypes import wintypes

# the app logs in Chinese; the Windows console's code page can't print it
sys.stdout.reconfigure(encoding="utf-8", errors="replace")
binary = sys.argv[1]
data = tempfile.mkdtemp(prefix="health-trail-app-smoke-")
with socket.socket() as s:
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
base = f"http://127.0.0.1:{port}"
args = [binary, "app", "-data", data, "-port", str(port)]
# a windowsgui exe has no console: its log goes where it's handed
log_path = os.path.join(data, "app.log")
log = open(log_path, "wb")
app = subprocess.Popen(args, stdout=log, stderr=subprocess.STDOUT)


def show_log():
    log.flush()
    with open(log_path, encoding="utf-8", errors="replace") as f:
        print("app log:\n" + f.read()[:6000])


def fail(msg):
    print("FAIL:", msg)
    show_log()
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

# the window: WebView2 sets up in the background, and quitting before it's
# done ends with a fatal "operation aborted" (exit 1). Nothing outside shows
# when it's done, so give it time and check it didn't fail.
time.sleep(15)
log.flush()
with open(log_path, encoding="utf-8", errors="replace") as f:
    if "WebView2 Error" in f.read():
        fail("the window's WebView2 failed")
if app.poll() is not None:
    fail(f"the app exited with {app.returncode} after starting")
print("ok  window")

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


def main_thread_window(pid):
    """Wails's hidden "__wails_hidden_mainthread" window of process pid."""
    user32 = ctypes.windll.user32
    found = []

    @ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)
    def each(hwnd, _):
        owner = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(owner))
        title = ctypes.create_unicode_buffer(64)
        user32.GetWindowTextW(hwnd, title, 64)
        if owner.value == pid and title.value == "__wails_hidden_mainthread":
            found.append(hwnd)
        return True

    user32.EnumWindows(each, 0)
    return found[0] if found else None


# quitting: the process is gone, the port free
hwnd = main_thread_window(app.pid)
if not hwnd:
    fail("no Wails main-thread window to ask to quit")
WM_CLOSE = 0x0010
ctypes.windll.user32.PostMessageW(hwnd, WM_CLOSE, 0, 0)
try:
    app.wait(30)
except subprocess.TimeoutExpired:
    fail("the app didn't quit within 30s of being asked (an update's restart would wait forever)")
if app.returncode != 0:
    fail(f"the app quit with {app.returncode}")
print("ok  quit")
with socket.socket() as s:
    try:
        s.bind(("127.0.0.1", port))
    except OSError:
        fail(f"port {port} still taken after quitting")
print("ok  port free")
