"""Private, bounded multi-format clipboard backup around native acceptance."""
from contextlib import AbstractContextManager
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import uuid


class NativeClipboardFixture(AbstractContextManager):
    def __init__(self, temporary, expected_path=None, nonce=None):
        self.temporary = Path(temporary)
        self.snapshot = self.temporary / "clipboard-original.plist"
        self.marker = self.temporary / "reasonix-native-clipboard-owned.json"
        self.binary = self.temporary / "clipboard-snapshot"
        self.nonce = nonce or uuid.uuid4().hex
        self.expected_path = expected_path

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
        if self.expected_path is not None:
            control = self.temporary / 'reasonix-native-ui-clipboard-path.json'
            control.touch(mode=0o600)
            control.write_text(json.dumps({'nonce': self.nonce, 'path': str(self.expected_path)}))
            action, argument = 'capture-path', control
        if self.command(action, argument).returncode:
            raise RuntimeError("current clipboard cannot be completely preserved within acceptance bounds; no clipboard write was performed")
        (self.temporary / "reasonix-native-clipboard-control.json").write_text(json.dumps({"nonce": self.nonce}))
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
