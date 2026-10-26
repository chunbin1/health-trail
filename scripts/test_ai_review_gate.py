#!/usr/bin/env python3
"""Tests of ai_review_gate.py's rules, without GitHub: python3 -I scripts/test_ai_review_gate.py"""
import os, sys, unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from ai_review_gate import counted, names, passes, pick_pr  # noqa: E402


class Passes(unittest.TestCase):
    def test_the_passing_sentence(self):
        for body in ["## AI 审阅（abc1234）\n\n**结论：没有必须修的问题，可以合。**",
                     "**结论：没有必须修的问题，可以合。** 修复思路对：只认 IB Gateway。",  # cb-quant #48
                     "结论：没有必须修的问题，可以合",
                     "**结论：没有必须修，可以合。**",  # cb-quant, shorter
                     "**结论: 没有必须修的问题，可以合。**",
                     "**结论：可以合**（Comment；无挡合并项）",  # another reviewing agent, PR #13
                     "**结论：可以合**",
                     "结论：可以合（无挡合并项）",
                     "**结论：可以合（无挡合并项）**",
                     "**结论：没有必须修的问题，可以合并。**"]:
            self.assertTrue(passes(body), body)

    def test_anything_else(self):
        for body in ["**结论：有必须修的问题，先别合。**",
                     "**结论：不可以合**",
                     "**结论：有必须修的问题，修完再合（修完可以合）**",
                     "**结论：有两处必须修，修完就可以合**",
                     "**结论：暂不可合；修好后可以合**",
                     "**结论：没有必须修的问题，可以合吗？**",
                     "**结论：可以合吗？**",
                     "**结论：可以合，但要先修 install_error 那处**",
                     "**结论：修完就可以合**",
                     "**结论：不可以合**",
                     "**结论：暂不可合**（有一处必须修）",
                     "没有结论行，可以合",
                     ""]:
            self.assertFalse(passes(body), body)

    def test_only_the_first_conclusion(self):
        self.assertFalse(passes("**结论：有必须修的问题，先别合。**\n\n结论：没有必须修的问题，可以合。"))


class Names(unittest.TestCase):
    sha = "a898a37ef2d8a607f133d2798e758202e5ec29d5"

    def test_short_or_full(self):
        self.assertTrue(names("## AI 审阅（a898a37）", self.sha))
        self.assertTrue(names(f"## AI 审阅（head `{self.sha}`）", self.sha))
        self.assertTrue(names("## AI 审阅（a898a37，增量：85f7687 / a898a37）", self.sha))

    def test_other_or_too_short(self):
        self.assertFalse(names("## AI 审阅（85f7687）", self.sha))
        self.assertFalse(names("## AI 审阅（a898a3）", self.sha))
        self.assertFalse(names("## AI 审阅", self.sha))


class Counted(unittest.TestCase):
    def test_owner_standing_ai_reviews(self):
        r = lambda user, body, state="COMMENTED": {"user": {"login": user}, "body": body, "state": state}
        reviews = [r("chunbin1", "## AI 审阅（a898a37）"), r("someone", "## AI 审阅（a898a37）"),
                   r("chunbin1", "## AI 审阅（a898a37）", "DISMISSED"), r("chunbin1", "## 总评"),
                   r("chunbin1", None)]
        self.assertEqual(counted(reviews, "chunbin1"), reviews[:1])


class PickPR(unittest.TestCase):
    def pr(self, n, merged=True, base="main", merge_sha=None, state="closed", head="h"):
        return {"number": n, "merged_at": "2026-10-26T00:00:00Z" if merged else None, "base": {"ref": base},
                "merge_commit_sha": merge_sha, "state": state, "head": {"sha": head}}

    def test_the_one_it_merged(self):
        pulls = [self.pr(5, merge_sha="other"), self.pr(8, merge_sha="abc")]
        self.assertEqual(pick_pr(pulls, "abc")["number"], 8)

    def test_not_merged_into_main(self):
        self.assertIsNone(pick_pr([self.pr(3, base="dev"), self.pr(4, merged=False, state="open", head="abc")], "abc"))

    def test_prerelease_open_head(self):
        pulls = [self.pr(4, merged=False, state="open", head="abc"), self.pr(6, merged=False, state="open", head="zzz")]
        self.assertEqual(pick_pr(pulls, "abc", prerelease=True)["number"], 4)
        self.assertIsNone(pick_pr(pulls, "abd", prerelease=True))


if __name__ == "__main__":
    unittest.main()
