"""ZIP an unpacked Windows executable, retaining the EXE for automatic updates."""
import hashlib
from pathlib import Path
import sys
import zipfile


def package(binary):
    if binary.suffix != '.exe':
        raise ValueError('Expected an .exe file')
    archive = binary.with_suffix('.zip')
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as output:
        output.write(binary, binary.name)
    # Verify the bytes users will extract, not just the archive's CRC.
    with zipfile.ZipFile(archive) as output:
        if output.namelist() != [binary.name]:
            raise ValueError('Unexpected Windows ZIP contents')
        with binary.open('rb') as original, output.open(binary.name) as extracted:
            if hashlib.file_digest(original, 'sha256').digest() != hashlib.file_digest(extracted, 'sha256').digest():
                raise ValueError('Windows ZIP executable differs from the build')
    return archive


if __name__ == '__main__':
    if len(sys.argv) != 2:
        sys.exit('Usage: package-windows.py binary.exe')
    package(Path(sys.argv[1]))
