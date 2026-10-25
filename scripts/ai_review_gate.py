#!/usr/bin/env python3
"""Has the code been through an AI review that says it can be merged?

    python3 -I scripts/ai_review_gate.py pr <number>     a pull request, at its head commit
    python3 -I scripts/ai_review_gate.py commit <sha>    the PR that brought a commit into main
    python3 -I scripts/ai_review_gate.py prerelease <sha>  the same, or an open PR at that commit
                                                            (a test build: v1.3.0-test.1)

A review counts when the repository owner posted it on the PR, its first line
is `## AI 审阅（<the head commit, 7 or more characters>…`, and its 结论 line
says 可以合 (see RELEASING.md for the format). A push after the review needs a
new one. Uses `gh api`; the repository is $GITHUB_REPOSITORY, or the one gh
finds here. Exits 0 when it passes, 1 with the reason when it does not.
"""
import json, os, re, subprocess, sys

repo = os.environ.get("GITHUB_REPOSITORY") or subprocess.run(
    ["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
    capture_output=True, text=True, check=True).stdout.strip()
owner = repo.split("/")[0]


def api(path):
    out = subprocess.run(["gh", "api", "--paginate", "--slurp", f"repos/{repo}/{path}"],
                         capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"gh api {path} 失败：{out.stderr.strip()}")
    pages = json.loads(out.stdout)
    return [x for page in pages for x in page] if pages and isinstance(pages[0], list) else pages[0]


def fail(msg):
    print("没通过：" + msg)
    sys.exit(1)


def passes(body):
    """The 结论 line says it can be merged: 可以合, and not 不可以合 / 先别合 / 不能合."""
    for line in body.splitlines():
        if re.match(r"\W*结论[：:]", line):
            return "可以合" in line and not re.search("不可以合|别合|不能合", line)
    return False


def check_pr(number):
    pr = api(f"pulls/{number}")
    head = pr["head"]["sha"]
    mine = [r for r in api(f"pulls/{number}/reviews")
            if r["user"]["login"] == owner and (r.get("body") or "").startswith("## AI 审阅")]
    if not mine:
        fail(f"PR #{number} 还没有 AI 审阅。审完以「## AI 审阅（{head[:7]}）」开头发一条评审，见 RELEASING.md。")
    # the latest review that names the head commit is the one that counts
    for r in reversed(mine):
        first = r["body"].splitlines()[0]
        if any(len(s) >= 7 and head.startswith(s) for s in re.findall(r"[0-9a-f]{7,40}", first)):
            if passes(r["body"]):
                print(f"通过：PR #{number} 的 {head[:7]} 有 AI 审阅，结论可以合（{r['html_url']}）")
                return
            fail(f"PR #{number} 的 {head[:7]} 最近一次 AI 审阅结论不是「可以合」：{r['html_url']}")
    fail(f"PR #{number} 最新的提交 {head[:7]} 还没审过（审阅之后又推了提交）。再审一次，标题写这个提交号。")


def check_commit(sha, prerelease=False):
    pulls = api(f"commits/{sha}/pulls")
    prs = [p for p in pulls if p.get("merged_at") and p["base"]["ref"] == "main"]
    if not prs and prerelease:
        prs = [p for p in pulls if p["state"] == "open" and p["head"]["sha"] == sha]
    if not prs:
        where = "合并进 main 的，也不是哪个 PR 最新的提交" if prerelease else "从 PR 合并进 main 的"
        fail(f"{sha[:7]} 不是{where}，没有经过 AI 审阅，不能发布。")
    check_pr(prs[0]["number"])


if len(sys.argv) != 3 or sys.argv[1] not in ("pr", "commit", "prerelease"):
    sys.exit(__doc__)
mode, arg = sys.argv[1:]
if mode == "pr":
    check_pr(arg)
else:
    check_commit(arg, prerelease=mode == "prerelease")
