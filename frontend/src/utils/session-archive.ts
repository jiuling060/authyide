import { Api, GithubComChaitinMonkeyCodeBackendPkgTaskflowFileKind, type GithubComChaitinMonkeyCodeBackendPkgTaskflowFile } from "@/api/Api"

/**
 * Collects a session workspace from the VM file API and packs it into a
 * gzip-compressed tar archive the build machine can overlay onto the template.
 *
 * The tar writer stays deliberately small: ustar headers, regular files and
 * directories. Symlinks are skipped rather than packed, because a symlink in
 * the archive is a path-traversal channel and the build machine refuses them.
 */

export interface SessionArchive {
  blob: Blob
  sha256: string
  fileCount: number
  sizeBytes: number
}

export interface SessionArchiveProgress {
  phase: "scanning" | "packing" | "hashing" | "uploading" | "verifying"
  seen: number
  total: number
}

interface ArchiveEntry {
  name: string
  mode: number
  dir: boolean
  body: Uint8Array | null
}

// Directories that are build output or dependency caches. Packing them would
// push the archive past the storage limit without adding source value.
const EXCLUDED_DIRS = new Set([
  ".git",
  ".expo",
  ".gradle",
  ".idea",
  ".vscode",
  ".cache",
  ".pytest_cache",
  ".mypy_cache",
  "__pycache__",
  "node_modules",
  "Pods",
  "DerivedData",
  "build",
  "dist",
  "out",
  "coverage",
])

const BLOCK_SIZE = 512
const MAX_NAME = 100
const MAX_PREFIX = 155
const MAX_FILES = 20000
const MAX_SCAN_DEPTH = 2000

function padTo(len: number): number {
  return Math.ceil(len / BLOCK_SIZE) * BLOCK_SIZE
}

// octal renders value as a zero-padded octal string right-aligned in a field of
// `width` bytes, NUL-terminated. That is the ustar layout every reader expects.
function octal(value: number, width: number): Uint8Array {
  const field = new Uint8Array(width)
  const text = Math.max(0, value).toString(8)
  const start = Math.max(0, width - 1 - text.length)
  for (let i = 0; i < text.length; i++) {
    field[start + i] = text.charCodeAt(i)
  }
  return field
}

function writeAscii(target: Uint8Array, offset: number, text: string): void {
  const bytes = new TextEncoder().encode(text)
  const n = Math.min(bytes.length, target.length - offset)
  target.set(bytes.subarray(0, n), offset)
}

function tarHeader(entry: ArchiveEntry): Uint8Array {
  const header = new Uint8Array(BLOCK_SIZE)
  const name = splitName(entry.name)

  writeAscii(header, 0, name.base)
  header.set(octal(entry.mode, 8), 100)
  header.set(octal(0, 8), 108)
  header.set(octal(0, 8), 116)
  header.set(octal(entry.body ? entry.body.length : 0, 12), 124)
  header.set(octal(Math.floor(Date.now() / 1000), 12), 136)
  header.fill(0x20, 148, 156)
  header[156] = entry.dir ? 0x35 : 0x30
  writeAscii(header, 257, "ustar")
  writeAscii(header, 263, "00")
  if (name.prefix) {
    writeAscii(header, 345, name.prefix)
  }

  let sum = 0
  for (const byte of header) {
    sum += byte
  }
  header.set(octal(sum, 6), 148)
  header[154] = 0x00
  header[155] = 0x20
  return header
}

// ustar splits long names over the prefix and name fields. The split point is
// the last slash that keeps both halves inside their fields.
function splitName(path: string): { prefix: string; base: string } {
  if (path.length <= MAX_NAME) {
    return { prefix: "", base: path }
  }
  for (let i = path.length - 1; i >= 0; i--) {
    if (path[i] !== "/") continue
    const base = path.slice(i + 1)
    const prefix = path.slice(0, i)
    if (base.length <= MAX_NAME && prefix.length <= MAX_PREFIX) {
      return { prefix, base }
    }
  }
  return { prefix: "", base: path.slice(0, MAX_NAME) }
}

function tarStream(entries: ArchiveEntry[]): Uint8Array {
  const sizes = entries.map((entry) => BLOCK_SIZE + (entry.body ? padTo(entry.body.length) : 0))
  const total = sizes.reduce((sum, size) => sum + size, BLOCK_SIZE * 2)
  const out = new Uint8Array(total)
  let offset = 0
  for (const entry of entries) {
    out.set(tarHeader(entry), offset)
    offset += BLOCK_SIZE
    if (entry.body) {
      out.set(entry.body, offset)
      offset = padTo(offset + entry.body.length)
    }
  }
  return out
}

async function gzip(bytes: Uint8Array): Promise<Blob> {
  if (typeof CompressionStream === "undefined") {
    throw new Error("gzip is not available in this browser")
  }
  // A plain copy detaches the payload from a possibly shared backing buffer.
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  const stream = new Blob([copy.buffer]).stream().pipeThrough(new CompressionStream("gzip"))
  // No media type on purpose: S3 rejects a presigned PUT when an unsigned
  // Content-Type header is sent.
  return new Response(stream).blob()
}

async function sha256(blob: Blob): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("")
}

function joinPath(dir: string, name: string): string {
  return dir === "" || dir === "/" ? name : `${dir}/${name}`
}

// The file API wants "/" for the workspace root but archive members must not
// start with a slash, so the two path forms are kept apart.
function apiPath(rel: string): string {
  return rel === "" ? "/" : rel
}

async function listFolder(
  api: Api<unknown>,
  vmId: string,
  rel: string,
): Promise<GithubComChaitinMonkeyCodeBackendPkgTaskflowFile[]> {
  const resp = await api.api.v1UsersFoldersList({ id: vmId, path: apiPath(rel) })
  const envelope = resp.data as { code?: number; data?: GithubComChaitinMonkeyCodeBackendPkgTaskflowFile[] } | null
  if (!resp.ok || envelope?.code !== 0 || !Array.isArray(envelope?.data)) {
    throw new Error("failed to list the workspace")
  }
  return envelope.data.filter(
    (entry) => entry.name && entry.name !== "." && entry.name !== "..",
  )
}

interface ScanHit {
  files: string[]
  dirs: string[]
}

async function scanDir(
  api: Api<unknown>,
  vmId: string,
  rel: string,
  hits: ScanHit,
): Promise<void> {
  for (const entry of await listFolder(api, vmId, rel)) {
    const name = entry.name as string
    const child = joinPath(rel, name)
    if (entry.kind === GithubComChaitinMonkeyCodeBackendPkgTaskflowFileKind.FileKindDir) {
      if (!EXCLUDED_DIRS.has(name)) hits.dirs.push(child)
      continue
    }
    if (entry.kind === GithubComChaitinMonkeyCodeBackendPkgTaskflowFileKind.FileKindFile) {
      hits.files.push(child)
    }
  }
}

async function walk(
  api: Api<unknown>,
  vmId: string,
  root: string,
  onScan: (seen: number) => void,
): Promise<ScanHit> {
  const hits: ScanHit = { files: [], dirs: [] }
  let frontier: string[] = [root]
  let scanned = 0

  while (frontier.length > 0 && scanned < MAX_SCAN_DEPTH) {
    const next: string[] = []
    for (const dir of frontier) {
      if (hits.files.length + hits.dirs.length >= MAX_FILES) break
      const before = hits.dirs.length
      await scanDir(api, vmId, dir, hits)
      next.push(...hits.dirs.slice(before))
      onScan(hits.files.length + hits.dirs.length)
    }
    scanned += frontier.length
    frontier = next
  }
  return hits
}

async function readEntry(api: Api<unknown>, vmId: string, path: string): Promise<Uint8Array> {
  const resp = await api.api.v1UsersFilesDownloadRaw({ id: vmId, path })
  if (!resp.ok) {
    throw new Error(`failed to read ${path}`)
  }
  // Without a `format`, the client hands back the raw Response untouched.
  return new Uint8Array(await resp.arrayBuffer())
}

export async function packSessionWorkspace(
  api: Api<unknown>,
  vmId: string,
  root: string,
  onProgress: (progress: SessionArchiveProgress) => void,
): Promise<SessionArchive> {
  const hits = await walk(api, vmId, root, (seen) =>
    onProgress({ phase: "scanning", seen, total: 0 }),
  )
  const { files, dirs } = hits
  if (files.length + dirs.length === 0) {
    throw new Error("the workspace is empty")
  }
  if (files.length > MAX_FILES) {
    throw new Error(`the workspace holds more than ${MAX_FILES} files`)
  }

  const entries: ArchiveEntry[] = []
  for (const dir of dirs) {
    entries.push({ name: `${dir}/`, mode: 0o755, dir: true, body: null })
  }
  for (let i = 0; i < files.length; i++) {
    const rel = files[i]
    entries.push({ name: rel, mode: 0o644, dir: false, body: await readEntry(api, vmId, rel) })
    onProgress({ phase: "packing", seen: i + 1, total: files.length })
  }

  onProgress({ phase: "hashing", seen: 0, total: 0 })
  const blob = await gzip(tarStream(entries))
  return {
    blob,
    sha256: await sha256(blob),
    fileCount: files.length,
    sizeBytes: blob.size,
  }
}
