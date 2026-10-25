#!/usr/bin/env python3
"""End-to-end check of a built binary's stdio MCP, the way Claude runs it.

    python3 -I scripts/mcp_smoke.py <path to health-trail>

Starts `health-trail mcp` on a throwaway data dir and sends one request at a
time, waiting for each reply (LESSONS #1: piping them all into stdin at once
closes it before the replies come). Exits non-zero on the first failure.
"""
import json, os, subprocess, sys, tempfile

binary = sys.argv[1]
data = tempfile.mkdtemp(prefix="health-trail-smoke-")
p = subprocess.Popen([binary, "mcp", "-data", data], stdin=subprocess.PIPE,
                     stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
n = 0


def fail(msg):
    p.kill()
    print("FAIL:", msg, "\nstderr:", p.stderr.read()[-800:])
    sys.exit(1)


def call(method, params=None, notify=False):
    global n
    msg = {"jsonrpc": "2.0", "method": method}
    if params is not None:
        msg["params"] = params
    if not notify:
        n += 1
        msg["id"] = n
    p.stdin.write(json.dumps(msg) + "\n")
    p.stdin.flush()
    if notify:
        return None
    while True:
        line = p.stdout.readline()
        if not line:
            fail(f"server closed during {method}")
        r = json.loads(line)
        if r.get("id") == n:
            return r


def tool(name, args):
    r = call("tools/call", {"name": name, "arguments": args})
    if "result" not in r:
        fail(f"{name}: {r}")
    res = r["result"]
    text = res["content"][0]["text"]
    if res.get("isError"):
        fail(f"{name}: {text}")
    print(f"ok  {name}")
    return text


if "result" not in call("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                                       "clientInfo": {"name": "ci-smoke", "version": "0"}}):
    fail("initialize")
call("notifications/initialized", notify=True)
tools = {t["name"] for t in call("tools/list")["result"]["tools"]}
for want in ("health_overview", "add_record", "add_attachment", "get_record"):
    if want not in tools:
        fail(f"tools/list has no {want}")
print(f"ok  tools/list ({len(tools)})")

tool("add_record", {"type": "conditions", "fields": {"name": "CI 冒烟测试"}})
if "CI 冒烟测试" not in tool("health_overview", {}):
    fail("health_overview doesn't show the new condition")

rep = json.loads(tool("add_record", {"type": "reports", "fields": {"title": "CI 报告", "report_date": "today"}}))
pdf = os.path.join(tempfile.mkdtemp(prefix="health-trail-smoke-file-"), "报告.pdf")
with open(pdf, "w") as f:
    f.write("%PDF-1.1\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Kids[]/Count 0>>endobj\n"
            "trailer<</Root 1 0 R>>\n%%EOF\n")
tool("add_attachment", {"path": pdf, "type": "reports", "id": rep["id"]})
if "报告.pdf" not in tool("get_record", {"type": "reports", "id": rep["id"]}):
    fail("get_record doesn't list the attachment")

p.stdin.close()
p.wait(timeout=10)
print("MCP smoke test passed")
