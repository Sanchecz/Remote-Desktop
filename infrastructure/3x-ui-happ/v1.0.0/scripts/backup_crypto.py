#!/usr/bin/env python3
"""Encrypt or decrypt portable 3x-ui backups using AES-256-GCM.

File format (all binary):
  magic[8] | salt[16] | nonce[12] | PBKDF2 iterations[4, big endian]
  | ciphertext[n] | GCM tag[16]

The passphrase is read from XUI_BACKUP_PASSPHRASE when set; otherwise it is
requested without terminal echo. The passphrase is never written to the file.
"""

from __future__ import annotations

import argparse
import getpass
import os
import struct
import sys
from pathlib import Path

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC


MAGIC = b"XUIBK01\0"
SALT_SIZE = 16
NONCE_SIZE = 12
TAG_SIZE = 16
HEADER_SIZE = len(MAGIC) + SALT_SIZE + NONCE_SIZE + 4
DEFAULT_ITERATIONS = 600_000
CHUNK_SIZE = 1024 * 1024


def passphrase(confirm: bool) -> bytes:
    value = os.environ.get("XUI_BACKUP_PASSPHRASE")
    if value is None:
        value = getpass.getpass("Backup passphrase: ")
        if confirm:
            repeated = getpass.getpass("Repeat passphrase: ")
            if value != repeated:
                raise SystemExit("Passphrases do not match")
    if len(value) < 16:
        raise SystemExit("Passphrase must contain at least 16 characters")
    return value.encode("utf-8")


def derive_key(secret: bytes, salt: bytes, iterations: int) -> bytes:
    kdf = PBKDF2HMAC(
        algorithm=hashes.SHA256(),
        length=32,
        salt=salt,
        iterations=iterations,
    )
    return kdf.derive(secret)


def temporary_path(output: Path) -> Path:
    return output.with_name(output.name + ".part")


def encrypt(source: Path, output: Path, secret: bytes, iterations: int) -> None:
    if source.resolve() == output.resolve():
        raise SystemExit("Input and output paths must differ")
    if output.exists():
        raise SystemExit(f"Output already exists: {output}")

    output.parent.mkdir(parents=True, exist_ok=True)
    temp = temporary_path(output)
    salt = os.urandom(SALT_SIZE)
    nonce = os.urandom(NONCE_SIZE)
    header = MAGIC + salt + nonce + struct.pack(">I", iterations)
    key = derive_key(secret, salt, iterations)
    encryptor = Cipher(algorithms.AES(key), modes.GCM(nonce)).encryptor()
    encryptor.authenticate_additional_data(header)

    try:
        with source.open("rb") as reader, temp.open("xb") as writer:
            writer.write(header)
            while chunk := reader.read(CHUNK_SIZE):
                writer.write(encryptor.update(chunk))
            writer.write(encryptor.finalize())
            writer.write(encryptor.tag)
            writer.flush()
            os.fsync(writer.fileno())
        os.replace(temp, output)
    except Exception:
        temp.unlink(missing_ok=True)
        raise


def decrypt(source: Path, output: Path, secret: bytes) -> None:
    if source.resolve() == output.resolve():
        raise SystemExit("Input and output paths must differ")
    if output.exists():
        raise SystemExit(f"Output already exists: {output}")

    total_size = source.stat().st_size
    if total_size < HEADER_SIZE + TAG_SIZE:
        raise SystemExit("Encrypted backup is truncated")

    output.parent.mkdir(parents=True, exist_ok=True)
    temp = temporary_path(output)
    try:
        with source.open("rb") as reader:
            header = reader.read(HEADER_SIZE)
            if header[: len(MAGIC)] != MAGIC:
                raise SystemExit("Unsupported backup format")
            offset = len(MAGIC)
            salt = header[offset : offset + SALT_SIZE]
            offset += SALT_SIZE
            nonce = header[offset : offset + NONCE_SIZE]
            offset += NONCE_SIZE
            iterations = struct.unpack(">I", header[offset : offset + 4])[0]
            if iterations < 100_000 or iterations > 10_000_000:
                raise SystemExit("Invalid PBKDF2 iteration count")

            ciphertext_size = total_size - HEADER_SIZE - TAG_SIZE
            reader.seek(total_size - TAG_SIZE)
            tag = reader.read(TAG_SIZE)
            reader.seek(HEADER_SIZE)

            key = derive_key(secret, salt, iterations)
            decryptor = Cipher(algorithms.AES(key), modes.GCM(nonce, tag)).decryptor()
            decryptor.authenticate_additional_data(header)

            remaining = ciphertext_size
            with temp.open("xb") as writer:
                while remaining:
                    chunk = reader.read(min(CHUNK_SIZE, remaining))
                    if not chunk:
                        raise SystemExit("Encrypted backup is truncated")
                    remaining -= len(chunk)
                    writer.write(decryptor.update(chunk))
                writer.write(decryptor.finalize())
                writer.flush()
                os.fsync(writer.fileno())
        os.replace(temp, output)
    except InvalidTag as exc:
        temp.unlink(missing_ok=True)
        raise SystemExit("Authentication failed: wrong passphrase or damaged file") from exc
    except Exception:
        temp.unlink(missing_ok=True)
        raise


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    encrypt_parser = subparsers.add_parser("encrypt")
    encrypt_parser.add_argument("input", type=Path)
    encrypt_parser.add_argument("output", type=Path)
    encrypt_parser.add_argument(
        "--iterations", type=int, default=DEFAULT_ITERATIONS
    )

    decrypt_parser = subparsers.add_parser("decrypt")
    decrypt_parser.add_argument("input", type=Path)
    decrypt_parser.add_argument("output", type=Path)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    if args.command == "encrypt":
        if args.iterations < 100_000 or args.iterations > 10_000_000:
            raise SystemExit("--iterations must be between 100000 and 10000000")
        encrypt(args.input, args.output, passphrase(confirm=True), args.iterations)
    else:
        decrypt(args.input, args.output, passphrase(confirm=False))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except FileNotFoundError as exc:
        raise SystemExit(f"File not found: {exc.filename}") from exc
