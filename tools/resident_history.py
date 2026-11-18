"""Print settlement events for one resident from a TestLongFoodCycles recording."""

import argparse
import gzip
import json


def contains_person(value, resident):
    if isinstance(value, dict):
        if {"origin", "first", "count"} <= value.keys():
            origin, sep, ordinal = resident.rpartition("_a")
            if sep and ordinal.isascii() and ordinal.isdigit() and origin == value["origin"]:
                number = int(ordinal)
                if resident == f"{origin}_a{number:04d}" and value["first"] <= number < value["first"] + value["count"]:
                    return True
        return any(contains_person(v, resident) for v in value.values())
    if isinstance(value, list):
        return any(contains_person(v, resident) for v in value)
    return value == resident


def history(path, resident):
    with gzip.open(path, "rt", encoding="utf-8") as stream:
        for line in stream:
            step = json.loads(line)
            if "day" in step:
                continue
            for event in step["events"] or ():
                if contains_person(event, resident):
                    payload = event.get("payload", {})
                    for field in ("ranges", "workers"):
                        if field in payload:
                            payload[field] = [r for r in payload[field] if contains_person(r, resident)]
                    yield event


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("recording", help="path to steps.ndjson.gz")
    parser.add_argument("resident", help="exact resident ID")
    args = parser.parse_args()
    found = False
    for match in history(args.recording, args.resident):
        print(json.dumps(match, ensure_ascii=False))
        found = True
    if not found:
        parser.exit(1, f"No matching events for {args.resident}\n")
