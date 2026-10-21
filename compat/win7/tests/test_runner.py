"""Acceptance runs must never reuse a historical successful worker response."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[3]


class RunnerTest(unittest.TestCase):
    def test_repeated_task_gets_a_new_worker_identity(self):
        cache = ROOT / '.cache/win7/runner-tests'
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=cache) as directory:
            base = Path(directory)
            (base / 'artifacts').mkdir()
            (base / 'logs').mkdir()
            task = base / 'case.json'
            task.write_text(json.dumps({'id': 'same-case', 'command': ['fixture.exe']}))
            (base / 'logs/win7-result-old.json').write_text(json.dumps({'task_id': 'same-case', 'exit': 0}))
            output = base / 'out.json'
            process = subprocess.Popen([sys.executable, str(Path(__file__).with_name('run-guest.py')), str(task), '--bridge', str(base), '--output', str(output), '--timeout', '10'], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                queue = base / 'artifacts/task.json'
                deadline = time.monotonic() + 5
                while not queue.exists() and time.monotonic() < deadline:
                    time.sleep(0.02)
                posted = json.loads(queue.read_text())
                self.assertNotEqual(posted['id'], 'same-case')
                self.assertFalse(output.exists())
                (base / 'logs/win7-result-new.json').write_text(json.dumps({'task_id': posted['id'], 'exit': 0}))
                stdout, stderr = process.communicate(timeout=10)
                self.assertEqual(process.returncode, 0, stdout + stderr)
                self.assertEqual(json.loads(output.read_text())['task_id'], posted['id'])
            finally:
                if process.poll() is None:
                    process.terminate()
                    process.wait(timeout=5)


if __name__ == '__main__':
    unittest.main()
