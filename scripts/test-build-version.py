"""Check Make and CI version extraction without building the desktop app."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parent.parent
WORKFLOW = (ROOT / '.github/workflows/build-and-release.yml').read_text()
TAG_SCRIPT = textwrap.dedent(WORKFLOW.split('        run: |\n')[1].split('\n  build:')[0])


class BuildVersionTest(unittest.TestCase):
    def test_release_title(self):
        command = WORKFLOW.split('        run: gh release create ', 1)[1].splitlines()[0]
        for tag, title in [('v0/v0.7.8', 'v0.7.8'),
                           ('v12/v12.3.4-rc.1', 'v12.3.4-rc.1'),
                           ('v0.7.8', 'v0.7.8')]:
            with self.subTest(tag=tag):
                result = subprocess.check_output(
                    ['bash', '-eu', '-c',
                     'gh() { printf "%s\\n" "$@"; }; gh release create ' + command],
                    env={**os.environ, 'TAG': tag}, text=True).splitlines()
                self.assertEqual(result[2], tag)
                self.assertEqual(result[result.index('--title') + 1], title)

    def test_versions(self):
        for tag, version in [('', None), ('v0/v0.7.1', '0.7.1'),
                             ('v12/v12.3.4-rc.1', '12.3.4'),
                             ('v0.7.1', '0.7.1'), ('other', None)]:
            with self.subTest(tag=tag), tempfile.TemporaryDirectory() as directory:
                def git(*args):
                    return subprocess.check_output(['git', '-C', directory, *args], text=True).strip()
                git('init', '-q')
                git('-c', 'user.name=Test', '-c', 'user.email=test@example.com',
                    'commit', '-q', '--allow-empty', '-m', 'Test')
                if tag:
                    git('tag', tag)
                expected = version or git('rev-parse', '--short=12', 'HEAD')
                result = subprocess.check_output(
                    ['make', '-s', '-f', str(ROOT / 'Makefile'),
                     '--eval=print-version:;@echo $(VERSION)', 'print-version'],
                    cwd=directory, text=True).strip()
                self.assertEqual(result, expected)
                output = Path(directory) / 'output'
                subprocess.run(['bash', '-eu', '-c', TAG_SCRIPT], check=True,
                               env={**os.environ, 'TAG': tag, 'REF_TYPE': 'tag' if tag else 'branch',
                                    'GITHUB_OUTPUT': str(output)})
                self.assertEqual(output.read_text(),
                                 f'tag={tag}\nversion={version}\nis_release=true\n' if version
                                 else 'is_release=false\n')


if __name__ == '__main__':
    unittest.main()
