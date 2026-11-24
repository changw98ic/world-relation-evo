import gzip
import json
import tempfile
import unittest
from pathlib import Path

from resident_history import history


class ResidentHistoryTest(unittest.TestCase):
    def test_worker_and_meal_ranges_include_only_the_requested_person(self):
        ranges = [dict(origin=o, first=8, count=3) for o in ("upstream", "village_a")]
        steps = [{"day": 1}, {"events": None}, {"events": [
            {"kind": "growth_work_completed", "payload": {"workers": ranges}},
            {"kind": "cohort_meals_settled", "payload": {"ranges": ranges}},
            {"kind": "personal_ration_settled", "actor_id": "village_a_a0009"},
        ]}]
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "steps.ndjson.gz"
            with gzip.open(path, "wt") as stream:
                for step in steps:
                    print(json.dumps(step), file=stream)
            for n in (8, 9, 10):
                events = list(history(path, f"village_a_a{n:04d}"))
                self.assertEqual(len(events), 3 if n == 9 else 2)
                for event, field in zip(events, ("workers", "ranges")):
                    self.assertEqual(event["payload"][field], ranges[1:])
            for n in (7, 11):
                self.assertEqual(list(history(path, f"village_a_a{n:04d}")), [])
