"""Resolve the release tag without changing checksum-pinned source inputs."""
import os
import re

PATTERN = re.compile(r'^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$')


def normalize(value):
    value = value.removeprefix('v')
    match = PATTERN.fullmatch(value)
    if not match or len(value) > 80:
        raise ValueError('Expected a semantic release version, e.g. v0.10.1 or v0.10.1-beta.1')
    if match[4] and any(p.isdecimal() and len(p) > 1 and p.startswith('0') for p in match[4].split('.')):
        raise ValueError('Numeric prerelease identifiers must not have leading zeroes')
    return value


def app_version(lock):
    return normalize(os.environ.get('CSGCLAW_WIN7_VERSION') or lock['app_version'])
