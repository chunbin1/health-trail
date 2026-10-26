#!/usr/bin/env python3
"""Has the code been through an AI review that says it can be merged?

    python3 -I scripts/ai_review_gate.py pr <number>     a pull request, at its head commit
    python3 -I scripts/ai_review_gate.py commit <sha>    the PR that brought a commit into main
    python3 -I scripts/ai_review_gate.py prerelease <sha>  the same, or an open PR at that commit
                                                            (a test build: v1.3.0-test.1)

A review counts when the repository owner posted it on the PR (and did not
dismiss it), its first line is `## AI 审阅（<the head commit, 7 or more
characters>…`, and its 结论 line says it can be merged with nothing to fix
first: 「结论：没有必须修的问题，可以合。」 (的问题 may go), or 「结论：可以合」 with
at most a note in brackets after it, as another reviewing agent writes it
(see RELEASING.md for the format). A push after the review needs a new one.
Uses `gh api`; the repository is $GITHUB_REPOSITORY, or the one gh finds here.
Exits 0 when it passes, 1 with the reason when it does not.
"""
import json, os, re, subprocess, sys

# The 结论 lines that let a PR through, bold or not. The long one says there
# is nothing to fix, so the reviewer's comment may follow the closing **:
PASS = re.compile(r"\**\s*结论[：:]\s*没有必须修(的问题)?，可以合并?[。.]?\s*(\*\*|$)")
# The short one, as another reviewing agent writes it, says only 可以合 (or
# 可以合并), so the line has to end there, but for one note in brackets with
# no condition in it: 「可以合**（Comment；无挡合并项）」, not 「可以合** 修完再合」
# or 「可以合（但先修 X）」. A note that only looks like a condition fails too.
SHORT = re.compile(r"\**\s*结论[：:]\s*可以合并?[。.]?\s*(\*\*)?\s*(?:[（(]([^（()）]*)[)）])?\s*(\*\*)?\s*$")
CONDITION = re.compile(r"修|但|先|再|后|前提|除非|条件|不|否|暂|等|吗|[?？]"
                       r"|\b(pending|but|after|until|unless|if|once|wait)\b", re.I)
# a 结论 line, 「**结论**：」 too: the first one is the review's, even when
# it isn't a passing one (a 结论 further down doesn't count)
CONCLUSION = re.compile(r"\W*结论(\*\*)?\s*[：:]")


def passes(body):
    """The review's 结论 line is a passing one."""
    for line in body.splitlines():
        if CONCLUSION.match(line):
            if PASS.match(line):
                return True
            m = SHORT.match(line)
            return bool(m) and not CONDITION.search(m.group(2) or "")
    return False


def names(first_line, sha):
    """The review's first line names the commit (7 or more of its characters)."""
    return any(sha.startswith(s) for s in re.findall(r"[0-9a-f]{7,40}", first_line))


def counted(reviews, owner):
    """The owner's AI reviews that still stand, oldest first."""
    return [r for r in reviews if r["user"]["login"] == owner and r.get("state") != "DISMISSED"
            and (r.get("body") or "").startswith("## AI 审阅")]


def pick_pr(pulls, sha, prerelease=False):
    """The PR that brought sha into main: the one whose merge commit it is first.
    For a test build, else the open PR whose latest commit it is."""
    merged = [p for p in pulls if p.get("merged_at") and p["base"]["ref"] == "main"]
    merged.sort(key=lambda p: p.get("merge_commit_sha") != sha)
    if merged:
        return merged[0]
    if prerelease:
        return next((p for p in pulls if p["state"] == "open" and p["head"]["sha"] == sha), None)
    return None


class Gate:
    def __init__(self):
        self.repo = os.environ.get("GITHUB_REPOSITORY") or subprocess.run(
            ["gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
            capture_output=True, text=True, check=True).stdout.strip()
        self.owner = self.repo.split("/")[0]

    def api(self, path):
        out = subprocess.run(["gh", "api", "--paginate", "--slurp", f"repos/{self.repo}/{path}"],
                             capture_output=True, text=True)
        if out.returncode != 0:
            sys.exit(f"gh api {path} 失败：{out.stderr.strip()}")
        pages = json.loads(out.stdout)
        return [x for page in pages for x in page] if pages and isinstance(pages[0], list) else pages[0]

    def pr(self, number):
        head = self.api(f"pulls/{number}")["head"]["sha"]
        mine = counted(self.api(f"pulls/{number}/reviews"), self.owner)
        if not mine:
            fail(f"PR #{number} 还没有 AI 审阅。审完以「## AI 审阅（{head[:7]}）」开头发一条评审，见 RELEASING.md。")
        # the latest review that names the head commit is the one that counts
        for r in reversed(mine):
            if names(r["body"].splitlines()[0], head):
                if passes(r["body"]):
                    print(f"通过：PR #{number} 的 {head[:7]} 有 AI 审阅，结论可以合（{r['html_url']}）")
                    return
                fail(f"PR #{number} 的 {head[:7]} 最近一次 AI 审阅结论不是「没有必须修的问题，可以合」或「可以合」（写法见 RELEASING.md）：{r['html_url']}")
        fail(f"PR #{number} 最新的提交 {head[:7]} 还没审过（审阅之后又推了提交）。再审一次，标题写这个提交号。")

    def commit(self, sha, prerelease=False):
        p = pick_pr(self.api(f"commits/{sha}/pulls"), sha, prerelease)
        if not p:
            where = "合并进 main 的，也不是哪个 PR 最新的提交" if prerelease else "从 PR 合并进 main 的"
            fail(f"{sha[:7]} 不是{where}，没有经过 AI 审阅，不能发布。")
        self.pr(p["number"])


def fail(msg):
    print("没通过：" + msg)
    sys.exit(1)


if __name__ == "__main__":
    if len(sys.argv) != 3 or sys.argv[1] not in ("pr", "commit", "prerelease"):
        sys.exit(__doc__)
    mode, arg = sys.argv[1:]
    if mode == "pr":
        Gate().pr(arg)
    else:
        Gate().commit(arg, prerelease=mode == "prerelease")
