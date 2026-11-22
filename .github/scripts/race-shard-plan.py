"""Plan the internal/game race shards by measured test time.

Round-robin let a few heavy tests (each booting the world many times under
-race) pile into one shard past the 15m timeout while others idled. The plan
is greedy longest-first: each test, heaviest first, goes to the least loaded
shard. Every shard computes the same plan and prints its own tests.

  race-shard-plan.py SHARD SHARDS WEIGHTS < listing      -> this shard's tests
  race-shard-plan.py --weights-from-logs LOG... > WEIGHTS -> refresh weights

Weights are seconds per top-level test from `-test.v` race runs; a test not
in the file weighs the mean of the listed ones.
"""

import re
import sys

NAME = re.compile(r"^(Test|Example|Fuzz)\S*$")
PASSED = re.compile(r"^--- (?:PASS|FAIL|SKIP): (\S+) \((\d+(?:\.\d+)?)s\)$")


def read_weights(path):
    weights = {}
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            name, seconds = line.split()
            weights[name] = float(seconds)
    return weights


def plan(names, shards, weights):
    known = [weights[n] for n in names if n in weights]
    default = sum(known) / len(known) if known else 1.0
    weight = {n: weights.get(n, default) for n in names}
    loads, owner = [0.0] * shards, {}
    for name in sorted(names, key=lambda n: (-weight[n], n)):
        shard = min(range(shards), key=lambda s: (loads[s], s))
        owner[name] = shard
        loads[shard] += weight[name]
    return owner


def weights_from_logs(paths):
    seconds = {}
    for path in paths:
        with open(path) as f:
            for line in f:
                m = PASSED.match(line.rstrip("\n"))
                if m and "/" not in m.group(1):
                    seconds[m.group(1)] = float(m.group(2))
    print("# Seconds per internal/game test under -race; refresh with")
    print("# race-shard-plan.py --weights-from-logs <-test.v race logs>.")
    for name in sorted(seconds):
        print(f"{name} {seconds[name]:.2f}")


def main(argv):
    if len(argv) >= 2 and argv[1] == "--weights-from-logs":
        weights_from_logs(argv[2:])
        return 0
    if len(argv) != 4 or not re.fullmatch(r"0|[1-9][0-9]*", argv[1]) or not re.fullmatch(r"[1-9][0-9]*", argv[2]):
        print("usage: race-shard-plan.py SHARD SHARDS WEIGHTS < listing", file=sys.stderr)
        return 2
    shard, shards = int(argv[1]), int(argv[2])
    if shard >= shards:
        print("shard index must be smaller than shard count", file=sys.stderr)
        return 2
    names = [line.strip() for line in sys.stdin if NAME.match(line.strip())]
    owner = plan(names, shards, read_weights(argv[3]))
    for name in names:
        if owner[name] == shard:
            print(name)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
