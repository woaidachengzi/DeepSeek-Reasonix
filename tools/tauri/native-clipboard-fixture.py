"""Private, bounded multi-format clipboard backup around native acceptance."""
from contextlib import AbstractContextManager
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import uuid


class NativeClipboardFixture(AbstractContextManager):
    def __init__(self, temporary, expected_path=None, nonce=None, image_format=None):
        self.temporary = Path(temporary)
        self.snapshot = self.temporary / "clipboard-original.plist"
        self.marker = self.temporary / "reasonix-native-clipboard-owned.json"
        self.binary = self.temporary / "clipboard-snapshot"
        self.nonce = nonce or uuid.uuid4().hex
        self.expected_path = expected_path
        if image_format not in (None, 'png', 'tiff') or (image_format and expected_path is not None):
            raise ValueError('Invalid private clipboard image mode')
        self.image_format = image_format

    def command(self, action, argument):
        return subprocess.run([str(self.binary), action, str(self.snapshot), str(argument)],
                              capture_output=True, text=True, timeout=15)

    def __enter__(self):
        if not self.binary.is_file():
            result = subprocess.run(["/usr/bin/xcrun", "swiftc", str(Path(__file__).with_name("clipboard-snapshot.swift")),
                                     "-module-cache-path", str(self.temporary / "swift-module-cache"),
                                     "-o", str(self.binary)], capture_output=True, text=True, timeout=60)
            if result.returncode:
                raise RuntimeError("native clipboard snapshot helper could not compile")
        if self.command("selftest", self.marker).returncode:
            raise RuntimeError("clipboard restoration safeguards failed in a private named pasteboard; no system clipboard write performed")
        self.marker.unlink(missing_ok=True)
        action, argument = 'capture', self.nonce
        if self.image_format:
            control = self.temporary / 'reasonix-native-ui-clipboard-image.json'
            control.touch(mode=0o600, exist_ok=False)
            control.write_text(json.dumps({'nonce': self.nonce, 'format': self.image_format}))
            action, argument = 'capture-image', control
        if self.expected_path is not None:
            control = self.temporary / 'reasonix-native-ui-clipboard-path.json'
            control.touch(mode=0o600)
            control.write_text(json.dumps({'nonce': self.nonce, 'path': str(self.expected_path)}))
            action, argument = 'capture-path', control
        capture = self.command(action, argument)
        if capture.returncode:
            # Report only a fixed category, never helper stderr or clipboard
            # content. Ownership changes and failed format round-trips are
            # different prerequisites from an unavailable/bounded snapshot.
            reason = {
                1: "snapshot unavailable or exceeds acceptance bounds",
                2: "clipboard changed during snapshot",
                3: "original formats cannot round-trip exactly",
            }.get(capture.returncode, "snapshot helper failed")
            raise RuntimeError(f"current clipboard preflight refused: {reason} (exit {capture.returncode}); no clipboard write was performed")
        (self.temporary / "reasonix-native-clipboard-control.json").write_text(json.dumps({"nonce": self.nonce}))
        if self.image_format:
            try:
                if self.command('seed-image', self.marker).returncode:
                    raise RuntimeError('private image clipboard seed failed')
            except (OSError, subprocess.SubprocessError, RuntimeError):
                # __enter__ failures do not invoke the context manager's exit.
                # Seeding can have written the clipboard before timing out.
                self.__exit__(RuntimeError)
                raise RuntimeError('private image clipboard seed failed; restoration attempted') from None
        return self

    def pump(self, inspect_live):
        pass

    def verify(self):
        if self.command("verify", self.marker).returncode:
            raise RuntimeError("system clipboard did not retain the exact native test value and generation")

    def __exit__(self, exc_type, *_):
        try:
            result = self.command("restore", self.marker)
        except (OSError, subprocess.SubprocessError):
            result = None
        if result is None or result.returncode != 0:
            # A changed generation is deliberately left untouched, but its
            # original backup is still needed after interrupted UI acceptance.
            # Keep it outside caller cleanup roots before reporting failure.
            recovery = Path(tempfile.mkdtemp(prefix="reasonix-clipboard-recovery-"))
            shutil.copy2(self.snapshot, recovery / "original.plist")
            if self.marker.is_file():
                shutil.copy2(self.marker, recovery / "owned.json")
            if result is not None and result.returncode == 2:
                if exc_type is not None:
                    print(f"native clipboard preservation after failed phase: new contents preserved; original recovery snapshot retained at {recovery}")
                    return False
                raise RuntimeError(f"clipboard changed during acceptance; current contents preserved; original recovery snapshot retained at {recovery}")
            raise RuntimeError(f"clipboard restoration failed; private recovery snapshot retained at {recovery}")
        self.snapshot.unlink(missing_ok=True)
        if exc_type is not None:
            state = "original formats preserved" if result.returncode == 0 else "new contents preserved"
            print(f"native clipboard preservation after failed phase: {state}")
        return False
