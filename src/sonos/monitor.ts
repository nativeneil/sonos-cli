import { SonosClient } from './client.js';
import { Song } from '../ai/index.js';
import {
  blockTrack,
  extractTrackIdFromUri,
  shouldMonitorContinue,
} from '../storage/index.js';

export interface MonitorOptions {
  pollMs?: number;
  minPlaySeconds?: number;
  stallSeconds?: number;
  onUnplayable: (song: Song, trackId: string | null) => Promise<void>;
  onStateLog?: (state: unknown) => void;
  /** If true, check monitor-until file and exit when expired */
  useTimer?: boolean;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function parseTimeToSeconds(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value !== 'string') return null;
  const parts = value.split(':').map((part) => parseInt(part, 10));
  if (parts.some((p) => Number.isNaN(p))) return null;
  let seconds = 0;
  for (const part of parts) {
    seconds = seconds * 60 + part;
  }
  return seconds;
}

interface TrackInfo {
  song: Song | null;
  positionSeconds: number | null;
  trackId: string | null;
  album: string | null;
}

function extractTrack(state: any): TrackInfo {
  const current = state?.currentTrack || state?.currenttrack || null;
  const title =
    current?.title ??
    current?.trackName ??
    current?.name ??
    current?.track ??
    null;
  const artist =
    current?.artist ??
    current?.artistName ??
    current?.creator ??
    current?.albumArtist ??
    null;
  const album = current?.album ?? null;

  if (!title || !artist) {
    return { song: null, positionSeconds: null, trackId: null, album: null };
  }

  const positionSeconds = parseTimeToSeconds(
    current?.position ?? current?.positionSeconds ?? state?.position
  );

  // Extract track ID from URI
  const uri = current?.uri ?? current?.trackUri ?? current?.albumArtUri ?? '';
  const trackId = extractTrackIdFromUri(uri);

  return { song: { title, artist }, positionSeconds, trackId, album };
}

export function startPlaybackMonitor(
  client: SonosClient,
  room: string,
  options: MonitorOptions
): { stop: () => void; done: Promise<void> } {
  const pollMs = options.pollMs ?? 4000;
  const minPlayMs = (options.minPlaySeconds ?? 8) * 1000;
  const stallMs = (options.stallSeconds ?? 10) * 1000;
  const useTimer = options.useTimer ?? false;
  const encodedRoom = encodeURIComponent(room);

  let stopped = false;
  let lastSong: Song | null = null;
  let lastSongKey = '';
  let lastTrackId: string | null = null;
  let lastAlbum: string | null = null;
  let lastTrackStartAt = 0;
  let lastPosition: number | null = null;
  let lastPositionAt = 0;
  let startedPlaying = false;
  let sawStall = false;

  const done = (async (): Promise<void> => {
    while (!stopped) {
      // Check timer if enabled
      if (useTimer && !shouldMonitorContinue()) {
        stopped = true;
        break;
      }

      let state: any;
      try {
        state = await client.request<any>(`/${encodedRoom}/state`);
      } catch {
        await sleep(pollMs);
        continue;
      }

      if (options.onStateLog) {
        options.onStateLog(state);
      }

      const playbackState = String(state?.playbackState || '').toUpperCase();
      const { song, positionSeconds, trackId, album } = extractTrack(state);
      if (!song) {
        await sleep(pollMs);
        continue;
      }
      const key = `${song.artist.toLowerCase()}:::${song.title.toLowerCase()}`;
      const now = Date.now();

      if (!lastSong) {
        lastSong = song;
        lastSongKey = key;
        lastTrackId = trackId;
        lastAlbum = album;
        lastTrackStartAt = now;
        lastPosition = positionSeconds;
        lastPositionAt = now;
        startedPlaying = positionSeconds !== null && positionSeconds >= 3;
        sawStall = false;
        await sleep(pollMs);
        continue;
      }

      if (key !== lastSongKey) {
        const neverStarted = !startedPlaying;
        if (neverStarted || sawStall) {
          // Track failed - block it and notify callback
          if (lastTrackId) {
            blockTrack(lastTrackId, lastSong.artist, lastSong.title, lastAlbum ?? undefined);
          }
          await options.onUnplayable(lastSong, lastTrackId);
        }
        lastSong = song;
        lastSongKey = key;
        lastTrackId = trackId;
        lastAlbum = album;
        lastTrackStartAt = now;
        lastPosition = positionSeconds;
        lastPositionAt = now;
        startedPlaying = positionSeconds !== null && positionSeconds >= 3;
        sawStall = false;
        await sleep(pollMs);
        continue;
      }

      if (positionSeconds !== null) {
        if (lastPosition !== null && positionSeconds === lastPosition) {
          if (now - lastPositionAt > stallMs) {
            sawStall = true;
          }
        } else {
          if (positionSeconds >= 3) {
            startedPlaying = true;
          }
          lastPosition = positionSeconds;
          lastPositionAt = now;
        }
      }

      await sleep(pollMs);
    }
  })();

  return {
    stop: () => {
      stopped = true;
    },
    done,
  };
}
