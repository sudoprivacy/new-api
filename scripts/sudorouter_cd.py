# sudoapi: Guarded single-VM SudoRouter continuous deployment.
"""Dispatch identifiers to the installed VM worker; print only safe receipts."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def completed_release(result):
    messages = [v.get('message', '') for v in result.get('value', [])]
    records = [line.removeprefix('SUDOROUTER_RESULT=') for message in messages
               for line in message.splitlines() if line.startswith('SUDOROUTER_RESULT=')]
    if len(records) != 1:
        raise RuntimeError('VM did not return a terminal release receipt')
    receipt = json.loads(records[0])
    if receipt.get('phase') != 'succeeded':
        raise RuntimeError('Release did not succeed: ' + str(receipt.get('phase')))
    if not (receipt.get('already_running') or receipt.get('bridge_removed')):
        raise RuntimeError('Release reported success before bridge cleanup')
    if receipt.get('availability_failures', 0):
        raise RuntimeError('Release completed with failed public availability samples')
    return receipt


def main():
    revision = os.environ['RELEASE_REVISION']
    image = os.environ['RELEASE_IMAGE']
    bundle = os.environ['RELEASE_BUNDLE']
    run = os.environ['GITHUB_RUN_ID'] + '-' + os.environ['GITHUB_RUN_ATTEMPT']
    if (not re.fullmatch('[a-f0-9]{40}', revision)
            or not re.fullmatch('sudorouter.azurecr.io/new-api@sha256:[a-f0-9]{64}', image)
            or not re.fullmatch('[a-f0-9]{64}', bundle)
            or not re.fullmatch('[0-9]{1,20}-[0-9]{1,3}', run)):
        raise ValueError('Invalid release identifiers')
    head = subprocess.check_output(['gh', 'api', 'repos/' + os.environ['GITHUB_REPOSITORY']
                                    + '/git/ref/heads/sudo/rc24', '--jq', '.object.sha'], text=True).strip()
    if head != revision:
        raise RuntimeError('A newer source revision exists; refusing a stale deployment')
    script = f'''set -eu
umask 077
python3 /opt/sudorouter-deploy/start_sudorouter_release.py --image {image} --revision {revision} --run {run} --bundle {bundle}
python3 - <<'PY'
import json,os,time
from pathlib import Path
directory=Path('/data/new-api/releases/gha-{run}')
deadline=time.monotonic()+1050
while time.monotonic()<deadline:
 path=directory/'state.json'
 if path.exists():
  state=json.loads(path.read_text())
  complete=state.get('already_running') or 'availability_samples' in state
  if complete:
   fields=['phase','bridge_removed','already_running','availability_samples','availability_failures','at']
   print('SUDOROUTER_RESULT='+json.dumps({{k:state[k] for k in fields if k in state}}),flush=True)
   break
 time.sleep(5)
else:
 raise RuntimeError('Release observation timed out; detached rollback remains responsible')
PY
'''
    with tempfile.TemporaryDirectory(prefix='sudorouter-cd-') as folder:
        path = Path(folder) / 'dispatch.sh'
        path.write_text(script)
        result = subprocess.run(['az', 'vm', 'run-command', 'invoke', '-g', 'sudo', '-n', 'sudorouter_jp',
                                 '--command-id', 'RunShellScript', '--scripts', '@' + str(path), '-o', 'json'],
                                capture_output=True, timeout=1200)
        if result.returncode:
            raise RuntimeError('Azure deployment command failed; inspect the VM release directory')
        receipt = completed_release(json.loads(result.stdout))
    print(json.dumps(receipt))


if __name__ == '__main__':
    main()
