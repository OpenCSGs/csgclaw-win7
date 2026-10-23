#!/usr/bin/env python3
"""Submit a fixture to the existing loopback-only Win7 polling test bridge."""
import argparse
import json
from pathlib import Path
import time
import uuid

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('task', type=Path)
parser.add_argument('--bridge', type=Path, required=True)
parser.add_argument('--queue', choices=['task.json', 'task-pty.json'], default='task.json')
parser.add_argument('--timeout', type=int, default=1200)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
task = json.loads(args.task.read_text())
assert task['id']
task['id'] += '-' + uuid.uuid4().hex[:12]
queue = args.bridge / 'artifacts' / args.queue
pending = queue.with_suffix('.pending')
pending.write_text(json.dumps(task))
pending.replace(queue)
deadline = time.monotonic() + args.timeout
seen = set()
while time.monotonic() < deadline:
    for p in (args.bridge / 'logs').glob('win7-result-*.json'):
        if p in seen:
            continue
        try:
            result = json.loads(p.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        seen.add(p)
        if not isinstance(result, dict) or result.get('task_id') != task['id']:
            continue
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(result, indent=2) + '\n')
        print(json.dumps({k: result.get(k) for k in ['task_id', 'exit', 'pid', 'error', 'duration']}))
        raise SystemExit(0 if not result.get('error') and result.get('exit', 0) == 0 else 1)
    time.sleep(2)
raise SystemExit('Timed out waiting for native fixture: ' + task['id'])
