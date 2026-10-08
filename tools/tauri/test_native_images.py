"""Local image-provider request gates; no network, app or clipboard access."""
import base64
import copy
import hashlib
import http.client
import importlib.util
from pathlib import Path
import struct
import unittest
import zlib

spec = importlib.util.spec_from_file_location("native_images", Path(__file__).with_name("smoke-native-images.py"))
images = importlib.util.module_from_spec(spec)
spec.loader.exec_module(images)


def png(width=64, height=40):
    def chunk(kind, payload):
        return struct.pack(">I", len(payload)) + kind + payload + struct.pack(">I", zlib.crc32(kind + payload))
    header = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    raw = (b"\x00" + b"\x00\x01\x02\xff" * width) * height
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def request(data=None):
    return {"model": "alpha", "stream": True, "messages": [{"role": "user", "content": [
        {"type": "text", "text": "reasonix-native-image-owned @.reasonix/attachments/clipboard-owned.png"},
        {"type": "image_url", "image_url": {"url": "data:image/png;base64," + base64.b64encode(png() if data is None else data).decode()}},
    ]}]}


class ImageRequestTests(unittest.TestCase):
    def test_loopback_provider_reply_completion_and_bounded_failure_status(self):
        for body, accepted in ((request(), True), ({"model": "alpha", "stream": True, "messages": [
            {"role": "user", "content": "private-fixture-sentinel-do-not-log"}]}, False)):
            fixture = images.ImageProvider("owned")
            fixture.thread.start()
            connection = http.client.HTTPConnection("127.0.0.1", fixture.server.server_port, timeout=3)
            try:
                connection.request("POST", "/v1/chat/completions", images.json.dumps(body))
                if accepted:
                    response = connection.getresponse()
                    self.assertEqual(response.status, 200)
                    self.assertIn(b"[DONE]", response.read(65536))
                    self.assertTrue(fixture.completed.wait(timeout=1))
                    fixture.verify()
                    self.assertEqual(fixture.failure_status(), {"requests": 1, "error": None,
                                                              "imageAccepted": True, "replyCompleted": True})
                else:
                    with self.assertRaises(http.client.RemoteDisconnected):
                        connection.getresponse()
                    status = fixture.failure_status()
                    self.assertEqual(status["error"], "model did not receive multimodal user input")
                    self.assertNotIn("private-fixture-sentinel", images.json.dumps(status))
                    self.assertFalse(status["imageAccepted"])
                    self.assertFalse(status["replyCompleted"])
            finally:
                connection.close()
                fixture.close()

    def test_bounded_real_image_and_owned_prompt(self):
        path, digest = images.validate_request(request(), "owned")
        self.assertEqual(path, ".reasonix/attachments/clipboard-owned.png")
        self.assertEqual(digest, hashlib.sha256(png()).hexdigest())

    def test_rejects_wrong_prompt_missing_image_and_extra_images(self):
        original = request()
        variants = []
        for key, value in (("model", "wrong"), ("stream", False)):
            changed = copy.deepcopy(original)
            changed[key] = value
            variants.append(changed)
        changed = copy.deepcopy(original)
        changed["messages"][0]["content"].pop()
        variants.append(changed)
        changed = copy.deepcopy(original)
        changed["messages"][0]["content"].append(changed["messages"][0]["content"][1])
        variants.append(changed)
        changed = copy.deepcopy(original)
        changed["messages"][0]["content"][0]["text"] = "foreign @.reasonix/attachments/clipboard-owned.png"
        variants.append(changed)
        for variant in variants:
            with self.assertRaises(ValueError):
                images.validate_request(variant, "owned")

    def test_rejects_header_only_spoof_dimensions_crc_and_trailing_bytes(self):
        corrupt = bytearray(png())
        corrupt[35] ^= 1
        for data in (png()[:33], png(65), bytes(corrupt), png() + b"extra", b"not-png"):
            with self.subTest(length=len(data)), self.assertRaises(ValueError):
                images.validate_request(request(data), "owned")


if __name__ == "__main__":
    unittest.main()
