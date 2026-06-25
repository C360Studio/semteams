export interface ZipFile {
  path: string;
  content: string | Uint8Array;
}

interface EncodedZipFile {
  nameBytes: Uint8Array;
  contentBytes: Uint8Array;
  crc32: number;
  localHeaderOffset: number;
}

const textEncoder = new TextEncoder();
const utf8Flag = 0x0800;
const storeMethod = 0;
const minZipVersion = 20;
const dosTimeMidnight = 0;
const dosDateJan11980 = 33;
const maxUint16 = 0xffff;
const maxUint32 = 0xffffffff;

let crcTable: Uint32Array | null = null;

export function createZipBlob(files: ZipFile[]): Blob {
  const archive = createZipArchive(files);
  const buffer = new ArrayBuffer(archive.byteLength);
  new Uint8Array(buffer).set(archive);
  return new Blob([buffer], { type: "application/zip" });
}

export function createZipArchive(files: ZipFile[]): Uint8Array {
  if (files.length === 0) {
    throw new Error("zip archive requires at least one file");
  }
  if (files.length > maxUint16) {
    throw new Error("zip archive has too many files");
  }

  const encodedFiles: EncodedZipFile[] = [];
  const chunks: Uint8Array[] = [];
  const seenPaths = new Set<string>();
  let offset = 0;

  for (const file of files) {
    const path = normalizeZipPath(file.path);
    if (seenPaths.has(path)) {
      throw new Error(`duplicate zip path: ${path}`);
    }
    seenPaths.add(path);

    const nameBytes = textEncoder.encode(path);
    const contentBytes =
      typeof file.content === "string"
        ? textEncoder.encode(file.content)
        : file.content;

    if (nameBytes.length > maxUint16) {
      throw new Error(`zip path is too long: ${path}`);
    }
    if (contentBytes.length > maxUint32) {
      throw new Error(`zip file is too large: ${path}`);
    }

    const encoded: EncodedZipFile = {
      nameBytes,
      contentBytes,
      crc32: crc32(contentBytes),
      localHeaderOffset: offset,
    };
    encodedFiles.push(encoded);

    const header = localFileHeader(encoded);
    chunks.push(header, contentBytes);
    offset += header.length + contentBytes.length;
  }

  const centralDirectoryOffset = offset;
  for (const file of encodedFiles) {
    const header = centralDirectoryHeader(file);
    chunks.push(header);
    offset += header.length;
  }
  const centralDirectorySize = offset - centralDirectoryOffset;
  if (centralDirectoryOffset > maxUint32 || centralDirectorySize > maxUint32) {
    throw new Error("zip archive is too large");
  }

  chunks.push(
    endOfCentralDirectory(
      encodedFiles.length,
      centralDirectorySize,
      centralDirectoryOffset,
    ),
  );
  return concat(chunks);
}

function normalizeZipPath(path: string): string {
  const normalized = path.replace(/\\/g, "/");
  const segments = normalized.split("/");
  if (
    normalized === "" ||
    normalized.startsWith("/") ||
    normalized.includes("\0") ||
    segments.some((part) => part === "" || part === "." || part === "..") ||
    /^[A-Za-z]:$/.test(segments[0] ?? "")
  ) {
    throw new Error(`unsafe zip path: ${path}`);
  }
  return normalized;
}

function localFileHeader(file: EncodedZipFile): Uint8Array {
  const out = new Uint8Array(30 + file.nameBytes.length);
  writeUint32(out, 0, 0x04034b50);
  writeUint16(out, 4, minZipVersion);
  writeUint16(out, 6, utf8Flag);
  writeUint16(out, 8, storeMethod);
  writeUint16(out, 10, dosTimeMidnight);
  writeUint16(out, 12, dosDateJan11980);
  writeUint32(out, 14, file.crc32);
  writeUint32(out, 18, file.contentBytes.length);
  writeUint32(out, 22, file.contentBytes.length);
  writeUint16(out, 26, file.nameBytes.length);
  writeUint16(out, 28, 0);
  out.set(file.nameBytes, 30);
  return out;
}

function centralDirectoryHeader(file: EncodedZipFile): Uint8Array {
  const out = new Uint8Array(46 + file.nameBytes.length);
  writeUint32(out, 0, 0x02014b50);
  writeUint16(out, 4, minZipVersion);
  writeUint16(out, 6, minZipVersion);
  writeUint16(out, 8, utf8Flag);
  writeUint16(out, 10, storeMethod);
  writeUint16(out, 12, dosTimeMidnight);
  writeUint16(out, 14, dosDateJan11980);
  writeUint32(out, 16, file.crc32);
  writeUint32(out, 20, file.contentBytes.length);
  writeUint32(out, 24, file.contentBytes.length);
  writeUint16(out, 28, file.nameBytes.length);
  writeUint16(out, 30, 0);
  writeUint16(out, 32, 0);
  writeUint16(out, 34, 0);
  writeUint16(out, 36, 0);
  writeUint32(out, 38, 0);
  writeUint32(out, 42, file.localHeaderOffset);
  out.set(file.nameBytes, 46);
  return out;
}

function endOfCentralDirectory(
  fileCount: number,
  centralDirectorySize: number,
  centralDirectoryOffset: number,
): Uint8Array {
  const out = new Uint8Array(22);
  writeUint32(out, 0, 0x06054b50);
  writeUint16(out, 4, 0);
  writeUint16(out, 6, 0);
  writeUint16(out, 8, fileCount);
  writeUint16(out, 10, fileCount);
  writeUint32(out, 12, centralDirectorySize);
  writeUint32(out, 16, centralDirectoryOffset);
  writeUint16(out, 20, 0);
  return out;
}

function concat(chunks: Uint8Array[]): Uint8Array {
  const length = chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const out = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

function writeUint16(out: Uint8Array, offset: number, value: number) {
  out[offset] = value & 0xff;
  out[offset + 1] = (value >>> 8) & 0xff;
}

function writeUint32(out: Uint8Array, offset: number, value: number) {
  out[offset] = value & 0xff;
  out[offset + 1] = (value >>> 8) & 0xff;
  out[offset + 2] = (value >>> 16) & 0xff;
  out[offset + 3] = (value >>> 24) & 0xff;
}

function crc32(bytes: Uint8Array): number {
  const table = crcTable ?? (crcTable = buildCrcTable());
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc = (crc >>> 8) ^ table[(crc ^ byte) & 0xff];
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function buildCrcTable(): Uint32Array {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i += 1) {
    let c = i;
    for (let j = 0; j < 8; j += 1) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[i] = c >>> 0;
  }
  return table;
}
