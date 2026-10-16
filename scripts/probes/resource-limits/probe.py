"""Controlled child-only resource evidence; no application or real repository access."""
import argparse
import json
import os
import platform
import signal
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path

SOURCE = Path(__file__).resolve().parent
MODES = ('memory', 'file', 'aggregate', 'nofile', 'inherit', 'cpu', 'cpu-hard')


def run(command, *, timeout=10, env=None):
    started = time.monotonic()
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               start_new_session=True, env=env)
    timed_out = False
    try:
        stdout, _ = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        os.killpg(process.pid, signal.SIGKILL)
        stdout, _ = process.communicate(timeout=5)
    return process.returncode, stdout, timed_out, round(time.monotonic() - started, 3)


def classify(command, *, docker=False, env=None):
    code, raw, timed_out, elapsed = run(command, env=env)
    try:
        result = json.loads(raw)
    except (ValueError, UnicodeDecodeError):
        raise RuntimeError('probe returned no valid classified output') from None
    result.update(exit_code=code, supervisor_timeout=timed_out, elapsed_seconds=elapsed)
    signal_number = -code if code < 0 else code - 128 if docker and code >= 128 else 0
    result['exit_signal'] = signal_number
    return result


def validate(cases, linux=False):
    # Capability gaps are observations, not reasons to erase a platform's data.
    # A supervisor kill must never be counted as the kernel enforcing a limit.
    capabilities = {
        'address_space': cases['memory'].get('baseline_mapping_allowed') is True and cases['memory'].get('mmap_blocked') is True,
        'single_file': cases['file'].get('blocked') is True and cases['file'].get('bytes', 65537) <= 65536,
        'per_file_not_aggregate': cases['aggregate'].get('aggregate_exceeds_per_file_limit') is True,
        'descriptors': cases['nofile'].get('exhausted') is True,
        'exec_inherits': cases['inherit'].get('inherited') is True,
        'hard_raise_denied': cases['inherit'].get('unprivileged') is True and cases['inherit'].get('raise_denied') is True,
        'cpu_soft': not cases['cpu']['supervisor_timeout'] and cases['cpu']['exit_signal'] == signal.SIGXCPU,
        'cpu_hard': not cases['cpu-hard']['supervisor_timeout'] and cases['cpu-hard']['exit_signal'] == signal.SIGKILL,
    }
    for mode, value in cases.items():
        if not mode.startswith('cpu') and (value['exit_code'] != 0 or value['supervisor_timeout']):
            raise RuntimeError(f'{mode} child failed before producing capability evidence')
    if linux and not all(capabilities.values()):
        raise RuntimeError('Linux fixture did not establish all expected capabilities')
    return capabilities


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--docker', action='store_true')
    args = parser.parse_args()
    output = {'synthetic_only': True, 'production_executor_tested': False}
    with tempfile.TemporaryDirectory(prefix='aim-resource-probe-') as root:
        binary = str(Path(root) / 'limits-probe')
        compile_result = run(['cc', '-std=c11', '-O2', '-Wall', '-Wextra', '-Werror',
                              str(SOURCE / 'limits.c'), '-o', binary], timeout=60)
        if compile_result[0] != 0:
            raise RuntimeError('native compiler failed; diagnostic omitted')
        local = {mode: classify([binary, mode, root]) for mode in MODES}
        capabilities = validate(local, linux=platform.system() == 'Linux')
        output['local'] = {'system': platform.system(), 'cases': local, 'capabilities': capabilities}
        print('native resource classification complete', file=sys.stderr, flush=True)
    if args.docker:
        env = dict(os.environ)
        host = subprocess.check_output(['docker', 'context', 'inspect', '--format',
                                        '{{.Endpoints.docker.Host}}'], text=True).strip()
        if not host.startswith('unix://'):
            raise RuntimeError('only a local Unix Docker daemon is permitted')
        env.pop('DOCKER_HOST', None)
        env.pop('DOCKER_CONTEXT', None)
        docker = ['docker', '--host', host]
        tag = 'aim-resource-probe-' + uuid.uuid4().hex[:12]
        print('building local Linux fixture from isolated probe context', file=sys.stderr, flush=True)
        build = run(docker + ['build', '--tag', tag, str(SOURCE)], timeout=240, env=env)
        if build[0] != 0 or build[2]:
            raise RuntimeError('controlled Docker build failed; no container evidence available')
        image = subprocess.check_output(docker + ['image', 'inspect', tag, '--format', '{{.Id}}'], env=env, text=True).strip()
        cases = {}
        def container(arguments, entrypoint=None):
            name = 'aim-resource-case-' + uuid.uuid4().hex[:12]
            command = docker + ['run', '--rm', '--init', '--name', name, '--network', 'none', '--read-only',
                      '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--user', '10001:10001',
                      '--memory', '256m', '--pids-limit', '16', '--cpus', '1',
                      '--tmpfs', '/work:rw,nosuid,nodev,noexec,size=1048576,mode=1777']
            if entrypoint:
                command += ['--entrypoint', entrypoint]
            try:
                return classify(command + [image] + arguments, docker=True, env=env)
            finally:
                subprocess.run(docker + ['rm', '-f', name], env=env, stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, timeout=10, check=False)
        for mode in MODES:
            cases[mode] = container([mode, '/work'])
        capabilities = validate(cases, linux=True)
        git = container([], '/usr/local/bin/git-limits-probe')
        if git['exit_code'] != 0 or not git.get('prlimit_git_sha1_sha256_fixed_fetch'):
            raise RuntimeError('real Git under prlimit did not complete controlled reads')
        output['docker'] = {'system': 'Linux', 'image_id': image, 'cases': cases, 'capabilities': capabilities, 'git': git}
    print(json.dumps(output, sort_keys=True))


if __name__ == '__main__':
    main()
