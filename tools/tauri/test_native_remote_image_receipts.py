import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("remote_image_boundary", Path(__file__).with_name("smoke-native-remote-image-boundary.py"))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)
spec = importlib.util.spec_from_file_location("remote_image_positive", Path(__file__).with_name("smoke-native-remote-image-positive.py"))
positive = importlib.util.module_from_spec(spec)
spec.loader.exec_module(positive)


class ReceiptTests(unittest.TestCase):
    def receipt(self):
        return dict(ok=True, registeredWebViewIPC=True, unknownFieldRejected=True,
                    invalidSourceRejected=True, unknownHandleRejected=True,
                    positivePixels=False, clipboardTouched=False, sharedTranscriptUI=False)

    def test_accepts_only_the_rejection_slice(self):
        probe.validate_receipt(self.receipt())

    def test_no_boolean_or_scope_is_optional(self):
        for key in self.receipt():
            for value in (None, 0, 1, "true", not self.receipt()[key]):
                with self.subTest(key=key, value=value):
                    receipt = self.receipt()
                    receipt[key] = value
                    with self.assertRaises(RuntimeError):
                        probe.validate_receipt(receipt)
            receipt = self.receipt()
            del receipt[key]
            with self.assertRaises(RuntimeError):
                probe.validate_receipt(receipt)

    def test_refuses_non_objects(self):
        for receipt in (None, [], True, "private", 1):
            with self.assertRaises(RuntimeError):
                probe.validate_receipt(receipt)

    def test_positive_requires_pixels_without_overclaiming_clipboard_or_ui(self):
        receipt = {**self.receipt(), "positivePixels": True}
        positive.validate_receipt(receipt)
        for key in receipt:
            for value in (None, 0, 1, "true", not receipt[key]):
                with self.subTest(key=key, value=value):
                    with self.assertRaises(RuntimeError):
                        positive.validate_receipt({**receipt, key: value})
            missing = dict(receipt)
            del missing[key]
            with self.assertRaises(RuntimeError):
                positive.validate_receipt(missing)
        for value in (None, [], True, "private", 1):
            with self.assertRaises(RuntimeError):
                positive.validate_receipt(value)


if __name__ == "__main__":
    unittest.main()
