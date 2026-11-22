"""Exercise the CI entry point with a recorded Go command boundary.

Case table: each shard x test/example/fuzz; discovery/test failures; invalid
indices; empty discovery/selection; other packages only on shard zero; the
weighted plan (heavy tests apart, unknown tests at the mean, weights parsed
from race logs).
Invariant: every test runs exactly once with race detection and a timeout.
Persistence: N/A. No game code or fixtures are changed.
"""

import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("test-race-shard.sh").resolve()
PLANNER = Path(__file__).with_name("race-shard-plan.py").resolve()
NAMES = ["TestA", "TestAB", "TestC", "TestD", "ExampleRun", "FuzzParse", "TestE"]
PACKAGES = ["example/game", "example/game/internal/game", "example/game/internal/config"]


class RaceShardTest(unittest.TestCase):
    def run_shard(self, index, count=4, names=NAMES, fail="", weights=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            weights_file = root / "weights.txt"
            weights_file.write_text("".join(f"{n} {w}\n" for n, w in (weights or {}).items()))
            fake_go = root / "go"
            fake_go.write_text("""#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
with open(os.environ['COMMAND_LOG'], 'a') as log:
    log.write(json.dumps(args) + '\\n')
kind = ('discover' if '-list' in args else
        'packages' if args == ['list', './...'] else
        'import' if args[0] == 'list' else
        'test' if './internal/game' in args else 'other')
if kind == os.environ['FAIL_COMMAND']:
    sys.exit(7)
if kind == 'discover':
    print('\\n'.join(json.loads(os.environ['TEST_NAMES'])))
    print('ok  example/game/internal/game 0.001s')
elif kind == 'packages':
    print('\\n'.join(json.loads(os.environ['PACKAGES'])))
elif kind == 'import':
    print('example/game/internal/game')
""")
            fake_go.chmod(0o755)
            log = root / "commands.jsonl"
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       COMMAND_LOG=str(log), FAIL_COMMAND=fail,
                       TEST_NAMES=json.dumps(names), PACKAGES=json.dumps(PACKAGES),
                       RACE_TEST_WEIGHTS=str(weights_file))
            result = subprocess.run(["bash", str(SCRIPT), str(index), str(count)],
                                    env=env, capture_output=True, text=True)
            commands = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, commands

    def test_partition_covers_each_parent_once_and_other_packages_once(self):
        matches = []
        other_runs = []
        for index in range(4):
            with self.subTest(shard=index):
                result, commands = self.run_shard(index)
                self.assertEqual(result.returncode, 0, result.stderr)
                runs = [c for c in commands if c[0] == "test" and "-list" not in c]
                for command in runs:
                    self.assertIn("-race", command)
                    self.assertIn("-count=1", command)
                    self.assertEqual(command[command.index("-timeout") + 1], "15m")
                game = runs[0]
                pattern = game[game.index("-run") + 1]
                selected = [name for name in NAMES if re.search(pattern, name)]
                self.assertTrue(selected)
                self.assertFalse(re.search(pattern, "TestABExtra"))
                matches.extend(selected)
                other_runs.extend(runs[1:])
        self.assertCountEqual(matches, NAMES)
        self.assertEqual(len(other_runs), 1)
        self.assertEqual(other_runs[0][-2:], [PACKAGES[0], PACKAGES[2]])

    def shard_of(self, weights, count=4):
        owner = {}
        for index in range(count):
            result, commands = self.run_shard(index, count, weights=weights)
            self.assertEqual(result.returncode, 0, result.stderr)
            pattern = commands[1][commands[1].index("-run") + 1]
            for name in NAMES:
                if re.search(pattern, name):
                    self.assertNotIn(name, owner)
                    owner[name] = index
        self.assertCountEqual(owner, NAMES)
        return owner

    def test_heavy_tests_go_to_separate_shards(self):
        heavy = {"TestA": 300, "TestC": 290, "TestD": 280, "TestE": 270}
        owner = self.shard_of(heavy | {"TestAB": 1, "ExampleRun": 1, "FuzzParse": 1})
        self.assertEqual(sorted(owner[n] for n in heavy), [0, 1, 2, 3])
        # The light tests fill the least loaded shard, never doubling a heavy one.
        self.assertEqual({owner[n] for n in ("TestAB", "ExampleRun", "FuzzParse")}, {3})

    def test_plan_is_stable(self):
        weights = {"TestA": 5, "TestC": 50}
        self.assertEqual(self.shard_of(weights), self.shard_of(weights))

    def test_unknown_tests_weigh_the_mean(self):
        # The five unknown tests weigh the mean, 100: TestA's shard takes two
        # of them, the other shard three plus TestC.
        owner = self.shard_of({"TestA": 190, "TestC": 10}, count=2)
        self.assertNotEqual(owner["TestA"], owner["TestC"])
        under_a = [n for n in NAMES if owner[n] == owner["TestA"]]
        self.assertEqual(len(under_a), 3)

    def test_weights_from_logs_keep_top_level_tests(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / "race.log"
            log.write_text("--- PASS: TestA (12.50s)\n    --- PASS: TestA/sub (3.00s)\n"
                           "--- FAIL: TestB (0.01s)\nok  \texample\t1.0s\n")
            result = subprocess.run(["python3", str(PLANNER), "--weights-from-logs", str(log)],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            lines = [l for l in result.stdout.splitlines() if not l.startswith("#")]
            self.assertEqual(lines, ["TestA 12.50", "TestB 0.01"])

    def test_failures_propagate(self):
        for failure, command_count in [("discover", 1), ("test", 2), ("import", 3),
                                       ("packages", 4), ("other", 5)]:
            with self.subTest(failure=failure):
                result, commands = self.run_shard(0, fail=failure)
                self.assertEqual(result.returncode, 7, result.stderr)
                self.assertEqual(len(commands), command_count)

    def test_invalid_indices_never_invoke_go(self):
        for index, count in [(-1, 4), (4, 4), (0, 0), ("x", 4), (0, "x"), ("01", 4)]:
            with self.subTest(index=index, count=count):
                result, commands = self.run_shard(index, count)
                self.assertEqual(result.returncode, 2)
                self.assertEqual(commands, [])

    def test_empty_selection_cannot_run_all_tests(self):
        for index, names in [(0, []), (3, ["TestOnly"])]:
            with self.subTest(index=index, names=names):
                result, commands = self.run_shard(index, names=names)
                self.assertEqual(result.returncode, 1)
                self.assertEqual(len(commands), 1)
                self.assertIn("-list", commands[0])


if __name__ == "__main__":
    unittest.main()
