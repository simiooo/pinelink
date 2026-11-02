import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('flutter-offline-shell.py')

class FlutterReleaseMetadata(unittest.TestCase):
    def test_release_removes_metadata_before_precaching(self):
        with tempfile.TemporaryDirectory(prefix='pinelink-shell-') as scratch:
            root = Path(scratch).resolve()
            self.assertEqual(root.parent, Path(tempfile.gettempdir()).resolve())
            self.assertTrue(root.name.startswith('pinelink-shell-'))
            for name in ['index.html', 'flutter_bootstrap.js', 'main.dart.js', 'drift_worker.js']:
                (root / name).write_text('runtime content', encoding='utf-8')
            for name in ['drift_worker.js.deps', 'main.dart.js.map', 'drift_worker.dart']:
                (root / name).write_text('generated build metadata', encoding='utf-8')
            subprocess.run([sys.executable, str(SCRIPT), str(root)], check=True, capture_output=True)
            for name in ['drift_worker.js.deps', 'main.dart.js.map', 'drift_worker.dart']:
                self.assertFalse((root / name).exists())
            self.assertEqual((root / 'drift_worker.js').read_text(), 'runtime content')
            worker = (root / 'pinelink-sw.js').read_text()
            self.assertNotIn('.deps', worker)
            self.assertNotIn('.js.map', worker)

    def test_source_directory_is_rejected_without_deleting_files(self):
        with tempfile.TemporaryDirectory(prefix='pinelink-shell-') as scratch:
            root = Path(scratch).resolve()
            self.assertEqual(root.parent, Path(tempfile.gettempdir()).resolve())
            self.assertTrue(root.name.startswith('pinelink-shell-'))
            source = root / 'drift_worker.dart'
            source.write_text('authored source', encoding='utf-8')
            result = subprocess.run([sys.executable, str(SCRIPT), str(root)], capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(source.read_text(), 'authored source')

if __name__ == '__main__':
    unittest.main()
