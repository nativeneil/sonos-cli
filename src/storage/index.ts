import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

const STORAGE_DIR = path.join(os.homedir(), '.sonos-playlist');

function ensureStorageDir(): void {
  if (!fs.existsSync(STORAGE_DIR)) {
    fs.mkdirSync(STORAGE_DIR, { recursive: true });
  }
}

// --- Blocklist ---

export interface BlockedTrack {
  trackId: string;
  artist: string;
  title: string;
  album?: string;
  replacementTrackId?: string;
  failedAt: string;
}

const BLOCKLIST_FILE = path.join(STORAGE_DIR, 'blocklist.json');

export function getBlocklist(): Map<string, BlockedTrack> {
  try {
    if (!fs.existsSync(BLOCKLIST_FILE)) {
      return new Map();
    }
    const data = JSON.parse(fs.readFileSync(BLOCKLIST_FILE, 'utf-8'));
    return new Map(Object.entries(data));
  } catch {
    return new Map();
  }
}

export function isTrackBlocked(trackId: number | string): boolean {
  const blocklist = getBlocklist();
  return blocklist.has(String(trackId));
}

export function blockTrack(
  trackId: string,
  artist: string,
  title: string,
  album?: string
): void {
  ensureStorageDir();
  const blocklist = getBlocklist();
  blocklist.set(trackId, {
    trackId,
    artist,
    title,
    album,
    failedAt: new Date().toISOString(),
  });
  const obj = Object.fromEntries(blocklist);
  fs.writeFileSync(BLOCKLIST_FILE, JSON.stringify(obj, null, 2));
}

export function setTrackReplacement(
  trackId: string,
  replacementTrackId: string
): void {
  ensureStorageDir();
  const blocklist = getBlocklist();
  const existing = blocklist.get(trackId);
  if (!existing) {
    return;
  }
  blocklist.set(trackId, {
    ...existing,
    replacementTrackId,
  });
  const obj = Object.fromEntries(blocklist);
  fs.writeFileSync(BLOCKLIST_FILE, JSON.stringify(obj, null, 2));
}

export function getSongReplacementTrackIds(song: {
  artist: string;
  title: string;
}): number[] {
  const normalizedArtist = song.artist.toLowerCase().trim();
  const normalizedTitle = song.title.toLowerCase().trim();
  const blocklist = getBlocklist();
  const replacements = new Set<number>();

  for (const blocked of blocklist.values()) {
    if (!blocked.replacementTrackId) {
      continue;
    }
    if (
      blocked.artist.toLowerCase().trim() === normalizedArtist &&
      blocked.title.toLowerCase().trim() === normalizedTitle
    ) {
      const id = parseInt(blocked.replacementTrackId, 10);
      if (!isNaN(id)) {
        replacements.add(id);
      }
    }
  }

  return [...replacements];
}

export function getBlockedTrackIds(): Set<number> {
  const blocklist = getBlocklist();
  const ids = new Set<number>();
  for (const key of blocklist.keys()) {
    const num = parseInt(key, 10);
    if (!isNaN(num)) {
      ids.add(num);
    }
  }
  return ids;
}

// --- Monitor Timer ---

const MONITOR_UNTIL_FILE = path.join(STORAGE_DIR, 'monitor-until.txt');

export function setMonitorUntil(timestamp: number): void {
  ensureStorageDir();
  fs.writeFileSync(MONITOR_UNTIL_FILE, String(timestamp));
}

export function getMonitorUntil(): number | null {
  try {
    if (!fs.existsSync(MONITOR_UNTIL_FILE)) {
      return null;
    }
    const data = fs.readFileSync(MONITOR_UNTIL_FILE, 'utf-8').trim();
    const ts = parseInt(data, 10);
    return isNaN(ts) ? null : ts;
  } catch {
    return null;
  }
}

export function extendMonitorBy(ms: number): void {
  const current = getMonitorUntil();
  const now = Date.now();
  const base = current && current > now ? current : now;
  setMonitorUntil(base + ms);
}

export function shouldMonitorContinue(): boolean {
  const until = getMonitorUntil();
  if (until === null) return false;
  return Date.now() < until;
}

// --- Track ID Extraction ---

export function extractTrackIdFromUri(uri: string): string | null {
  // URIs look like: x-sonosapi-hls-static:song%3a1445765901?sid=204...
  // or albumArtUri: /getaa?s=1&u=x-sonosapi-hls-static%3asong%253a1445765901...
  const match = uri.match(/song[:%]3[Aa]?(\d+)/);
  return match ? match[1] : null;
}
