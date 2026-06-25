import { describe, expect, it } from "vitest";
import { createZipArchive } from "./zip";

const decoder = new TextDecoder();

function readUint16(bytes: Uint8Array, offset: number): number {
  return bytes[offset] | (bytes[offset + 1] << 8);
}

function readUint32(bytes: Uint8Array, offset: number): number {
  return (
    (bytes[offset] |
      (bytes[offset + 1] << 8) |
      (bytes[offset + 2] << 16) |
      (bytes[offset + 3] << 24)) >>>
    0
  );
}

function parseStoredZip(bytes: Uint8Array): Record<string, string> {
  const files: Record<string, string> = {};
  let offset = 0;
  while (readUint32(bytes, offset) === 0x04034b50) {
    const compression = readUint16(bytes, offset + 8);
    expect(compression).toBe(0);
    const compressedSize = readUint32(bytes, offset + 18);
    const uncompressedSize = readUint32(bytes, offset + 22);
    expect(compressedSize).toBe(uncompressedSize);
    const nameLength = readUint16(bytes, offset + 26);
    const extraLength = readUint16(bytes, offset + 28);
    const nameStart = offset + 30;
    const dataStart = nameStart + nameLength + extraLength;
    const name = decoder.decode(bytes.slice(nameStart, nameStart + nameLength));
    files[name] = decoder.decode(
      bytes.slice(dataStart, dataStart + uncompressedSize),
    );
    offset = dataStart + compressedSize;
  }
  expect(readUint32(bytes, offset)).toBe(0x02014b50);
  return files;
}

describe("createZipArchive", () => {
  it("writes a portable stored ZIP with file paths and contents", () => {
    const zip = createZipArchive([
      {
        path: "openspec/changes/add-mfa/proposal.md",
        content: "# Proposal\n",
      },
      {
        path: "openspec/changes/add-mfa/tasks.md",
        content: "- [ ] 1.1 Build it\n",
      },
    ]);

    expect(readUint32(zip, 0)).toBe(0x04034b50);
    expect(readUint32(zip, zip.length - 22)).toBe(0x06054b50);
    expect(parseStoredZip(zip)).toEqual({
      "openspec/changes/add-mfa/proposal.md": "# Proposal\n",
      "openspec/changes/add-mfa/tasks.md": "- [ ] 1.1 Build it\n",
    });
  });

  it.each([
    "../escape.md",
    "/tmp/escape.md",
    "C:\\tmp\\escape.md",
    "openspec\\..\\escape.md",
  ])("rejects unsafe archive path %s", (path) => {
    expect(() => createZipArchive([{ path, content: "nope" }])).toThrow(
      "unsafe zip path",
    );
  });

  it("rejects duplicate archive paths", () => {
    expect(() =>
      createZipArchive([
        { path: "openspec/changes/add-mfa/proposal.md", content: "one" },
        { path: "openspec/changes/add-mfa/proposal.md", content: "two" },
      ]),
    ).toThrow("duplicate zip path");
  });
});
