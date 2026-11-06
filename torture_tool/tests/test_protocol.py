"""Unit test suite for Phase T0 protocol models, codecs, and golden fixtures."""

import json
import os
import unittest
from pathlib import Path

from torture_tool.protocol.codec import encode_request, parse_request, parse_response
from torture_tool.protocol.spec import (
    AckResponse,
    BatchItem,
    BatchRequest,
    DelRequest,
    ErrResponse,
    ErrorCode,
    MAX_RAW_VALUE_LEN,
    PingRequest,
    PongResponse,
    ProtocolParseError,
    ProtocolValidationError,
    PutRequest,
)
from torture_tool.protocol.utils import bytes_to_hex, hex_to_bytes


class TestProtocolUtils(unittest.TestCase):
    def test_hex_round_trip(self):
        sample = b"Hello, World!\x00\xff\xfe\x01\n\r\t"
        hex_str = bytes_to_hex(sample)
        self.assertEqual(hex_to_bytes(hex_str), sample)

    def test_empty_bytes_hex(self):
        self.assertEqual(bytes_to_hex(b""), "")
        self.assertEqual(hex_to_bytes(""), b"")

    def test_odd_length_hex_rejected(self):
        with self.assertRaises(ProtocolParseError):
            hex_to_bytes("123")

    def test_non_hex_characters_rejected(self):
        with self.assertRaises(ProtocolParseError):
            hex_to_bytes("123g")


class TestProtocolModels(unittest.TestCase):
    def test_valid_put(self):
        req = PutRequest(op_id="op-1", key=b"key1", value=b"val1")
        self.assertEqual(req.op_id, "op-1")
        self.assertEqual(req.key, b"key1")
        self.assertEqual(req.value, b"val1")

    def test_put_empty_key_rejected(self):
        with self.assertRaises(ProtocolValidationError):
            PutRequest(op_id="op-1", key=b"", value=b"val1")

    def test_put_empty_value_allowed(self):
        req = PutRequest(op_id="op-1", key=b"key1", value=b"")
        self.assertEqual(req.value, b"")

    def test_put_oversized_value_rejected(self):
        oversized = b"x" * (MAX_RAW_VALUE_LEN + 1)
        with self.assertRaises(ProtocolValidationError):
            PutRequest(op_id="op-1", key=b"key1", value=oversized)

    def test_invalid_op_id_rejected(self):
        with self.assertRaises(ProtocolValidationError):
            PutRequest(op_id="op with space", key=b"k", value=b"v")

    def test_batch_validation(self):
        item1 = BatchItem(key=b"k1", value=b"v1")
        item2 = BatchItem(key=b"k2", value=None)  # DEL
        batch = BatchRequest(op_id="b-1", items=(item1, item2))
        self.assertEqual(len(batch.items), 2)
        self.assertIsNone(batch.items[1].value)

    def test_batch_empty_items_rejected(self):
        with self.assertRaises(ProtocolValidationError):
            BatchRequest(op_id="b-1", items=())


class TestProtocolCodecs(unittest.TestCase):
    def test_encode_and_parse_put(self):
        req = PutRequest(op_id="w1-100", key=b"user:42", value=b"alice\x00data")
        wire = encode_request(req)
        self.assertTrue(wire.endswith("\n"))
        parsed = parse_request(wire)
        self.assertIsInstance(parsed, PutRequest)
        self.assertEqual(parsed.op_id, req.op_id)
        self.assertEqual(parsed.key, req.key)
        self.assertEqual(parsed.value, req.value)

    def test_encode_and_parse_del(self):
        req = DelRequest(op_id="w1-101", key=b"user:42")
        wire = encode_request(req)
        self.assertEqual(wire, f"DEL w1-101 {bytes_to_hex(b'user:42')}\n")
        parsed = parse_request(wire)
        self.assertIsInstance(parsed, DelRequest)
        self.assertEqual(parsed.op_id, req.op_id)
        self.assertEqual(parsed.key, req.key)

    def test_encode_and_parse_batch(self):
        items = (
            BatchItem(key=b"k1", value=b"v1"),
            BatchItem(key=b"k2", value=None),
            BatchItem(key=b"k3", value=b"v3"),
        )
        req = BatchRequest(op_id="batch-99", items=items)
        wire = encode_request(req)
        self.assertTrue(wire.startswith("BATCH batch-99 3 "))
        parsed = parse_request(wire)
        self.assertIsInstance(parsed, BatchRequest)
        self.assertEqual(len(parsed.items), 3)
        self.assertEqual(parsed.items[0].key, b"k1")
        self.assertEqual(parsed.items[0].value, b"v1")
        self.assertEqual(parsed.items[1].key, b"k2")
        self.assertIsNone(parsed.items[1].value)
        self.assertEqual(parsed.items[2].key, b"k3")
        self.assertEqual(parsed.items[2].value, b"v3")

    def test_encode_and_parse_ping(self):
        req = PingRequest(op_id="ping-01")
        wire = encode_request(req)
        self.assertEqual(wire, "PING ping-01\n")
        parsed = parse_request(wire)
        self.assertIsInstance(parsed, PingRequest)
        self.assertEqual(parsed.op_id, "ping-01")

    def test_parse_response_ack(self):
        resp = parse_response("ACK op-123\n")
        self.assertIsInstance(resp, AckResponse)
        self.assertEqual(resp.op_id, "op-123")
        self.assertIsNone(resp.seq_num)

        resp_seq = parse_response("ACK op-123 9999\n")
        self.assertIsInstance(resp_seq, AckResponse)
        self.assertEqual(resp_seq.op_id, "op-123")
        self.assertEqual(resp_seq.seq_num, 9999)

    def test_parse_response_err(self):
        resp = parse_response("ERR op-123 QUOTA_EXCEEDED Max disk size reached\n")
        self.assertIsInstance(resp, ErrResponse)
        self.assertEqual(resp.op_id, "op-123")
        self.assertEqual(resp.error_code, "QUOTA_EXCEEDED")
        self.assertEqual(resp.message, "Max disk size reached")

    def test_parse_response_pong(self):
        resp = parse_response("PONG ping-01\n")
        self.assertIsInstance(resp, PongResponse)
        self.assertEqual(resp.op_id, "ping-01")

    def test_parse_response_unknown_tag(self):
        with self.assertRaises(ProtocolParseError):
            parse_response("FOOBAR op-1\n")


class TestGoldenFixtures(unittest.TestCase):
    def setUp(self):
        fixture_path = (
            Path(__file__).parent / "fixtures" / "protocol_golden.json"
        )
        with open(fixture_path, "r", encoding="utf-8") as f:
            self.data = json.load(f)

    def test_valid_fixtures(self):
        for fixture in self.data["fixtures"]:
            f_type = fixture["type"]
            wire = fixture["wire"]
            model = fixture["model"]

            if f_type == "request":
                parsed = parse_request(wire)
                self.assertEqual(parsed.op_id, model["op_id"])
                if model["kind"] == "PUT":
                    self.assertIsInstance(parsed, PutRequest)
                    self.assertEqual(bytes_to_hex(parsed.key), model["key_hex"])
                    self.assertEqual(bytes_to_hex(parsed.value), model["val_hex"])
                    # Verify re-encoding produces identical wire format
                    re_encoded = encode_request(parsed)
                    self.assertEqual(re_encoded, wire)
                elif model["kind"] == "DEL":
                    self.assertIsInstance(parsed, DelRequest)
                    self.assertEqual(bytes_to_hex(parsed.key), model["key_hex"])
                    self.assertEqual(encode_request(parsed), wire)
                elif model["kind"] == "BATCH":
                    self.assertIsInstance(parsed, BatchRequest)
                    self.assertEqual(len(parsed.items), model["count"])
                    self.assertEqual(encode_request(parsed), wire)
                elif model["kind"] == "PING":
                    self.assertIsInstance(parsed, PingRequest)
                    self.assertEqual(encode_request(parsed), wire)

            elif f_type == "response":
                resp = parse_response(wire)
                self.assertEqual(resp.op_id, model["op_id"])
                if model["kind"] == "ACK":
                    self.assertIsInstance(resp, AckResponse)
                    self.assertEqual(resp.seq_num, model["seq_num"])
                elif model["kind"] == "ERR":
                    self.assertIsInstance(resp, ErrResponse)
                    self.assertEqual(resp.error_code, model["error_code"])
                    self.assertEqual(resp.message, model["message"])
                elif model["kind"] == "PONG":
                    self.assertIsInstance(resp, PongResponse)

    def test_invalid_fixtures(self):
        for invalid_line in self.data["invalid_lines"]:
            with self.subTest(invalid_line=invalid_line):
                # An invalid line must fail either request parsing or response parsing or both
                with self.assertRaises((ProtocolParseError, ProtocolValidationError)):
                    # Try parsing as response or request
                    try:
                        parse_response(invalid_line)
                    except (ProtocolParseError, ProtocolValidationError):
                        # also ensure parse_request fails
                        parse_request(invalid_line)


if __name__ == "__main__":
    unittest.main()

