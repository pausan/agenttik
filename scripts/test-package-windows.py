"""Check the portable Windows ZIP layout and preservation of executable bytes."""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location(
    'package_windows', Path(__file__).with_name('package-windows.py'))
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class WindowsPackageTest(unittest.TestCase):
    def test_package_and_rebuild(self):
        with tempfile.TemporaryDirectory(prefix='windows package ') as directory:
            binary = Path(directory) / 'agenttik_v0.7.14_windows_amd64.exe'
            for payload in [b'MZ' + bytes(range(256)) * 100, b'MZnew build']:
                binary.write_bytes(payload)
                archive = packager.package(binary)
                self.assertEqual(archive, binary.with_suffix('.zip'))
                self.assertEqual(binary.read_bytes(), payload)
                with zipfile.ZipFile(archive) as output:
                    self.assertEqual(output.namelist(), [binary.name])
                    self.assertEqual(output.read(binary.name), payload)
                    self.assertEqual(output.getinfo(binary.name).compress_type,
                                     zipfile.ZIP_DEFLATED)

    def test_reject_non_executable_path(self):
        with self.assertRaises(ValueError):
            packager.package(Path('agenttik.zip'))


if __name__ == '__main__':
    unittest.main()
