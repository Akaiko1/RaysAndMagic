"""Exercise the CI entry point with a recorded Go command boundary.

Case table: each shard x test/example/fuzz; discovery/test failures; invalid
indices; empty discovery/selection; other packages only on shard zero.
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
NAMES = ["TestA", "TestAB", "TestC", "TestD", "ExampleRun", "FuzzParse", "TestE"]
PACKAGES = ["example/game", "example/game/internal/game", "example/game/internal/config"]


class RaceShardTest(unittest.TestCase):
    def run_shard(self, index, count=4, names=NAMES, fail=""):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
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
                       TEST_NAMES=json.dumps(names), PACKAGES=json.dumps(PACKAGES))
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
                self.assertEqual(selected, NAMES[index::4])
                self.assertFalse(re.search(pattern, "TestABExtra"))
                matches.extend(selected)
                other_runs.extend(runs[1:])
        self.assertCountEqual(matches, NAMES)
        self.assertEqual(len(other_runs), 1)
        self.assertEqual(other_runs[0][-2:], [PACKAGES[0], PACKAGES[2]])

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
