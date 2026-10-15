# sudoapi: Native offline SudoRouter releases.
import gzip
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


class PackageTests(unittest.TestCase):
    def package(self, architecture, actual=None):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        binary = root / "bin"
        binary.mkdir()
        docker = binary / "docker"
        docker.write_text('''#!/bin/sh
echo "$*" >> "$DOCKER_LOG"
case "$1 $2" in
  'image inspect')
    case "$*" in *Architecture*) echo "linux/$ACTUAL_ARCH";; *) echo sha256:fixture;; esac;;
  'run -d') echo fixture-container;;
  'exec fixture-container') echo '{"success":true}';;
  'save '*) printf 'fixture image archive';;
esac
''')
        docker.chmod(0o755)
        env = {**os.environ, "PATH": str(binary) + os.pathsep + os.environ["PATH"],
               "ACTUAL_ARCH": actual or architecture, "DOCKER_LOG": str(root / "docker.log")}
        result = subprocess.run(["bash", str(Path(__file__).with_name("package-sudorouter.sh")),
                                 "sudorouter-v1.2.3", architecture, str(root / "output")],
                                env=env, capture_output=True, text=True)
        return root, result

    def test_both_architectures_emit_offline_archive_and_provenance(self):
        for arch in ("amd64", "arm64"):
            with self.subTest(arch=arch):
                root, result = self.package(arch)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                stem = f"sudorouter-1.2.3-linux-{arch}"
                self.assertEqual(gzip.decompress((root / f"output/{stem}.tar.gz").read_bytes()), b"fixture image archive")
                source = json.loads((root / f"output/{stem}.source.json").read_text())
                self.assertEqual(source["architecture"], arch)
                self.assertEqual(source["image"], f"docker.io/sudoprivacy/new-api:1.2.3-{arch}")
                subprocess.run(["shasum", "-a", "256", "-c", "SHA256SUMS"], cwd=root / "output", check=True, capture_output=True)
                self.assertIn("rm -f fixture-container", (root / "docker.log").read_text())

    def test_wrong_architecture_fails_before_smoke_or_export(self):
        root, result = self.package("arm64", "amd64")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("architecture differs", result.stderr)
        self.assertNotIn("run -d", (root / "docker.log").read_text())
        self.assertEqual(list((root / "output").iterdir()), [])
