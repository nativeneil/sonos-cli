import { SonosClient } from './client.js';
import { Song } from '../ai/index.js';
import {
  blockTrack,
  getBlockedTrackIds,
  getSongReplacementTrackIds,
} from '../storage/index.js';

export interface QueueResult {
  song: Song;
  success: boolean;
  error?: string;
  trackId?: number;
}

interface iTunesSearchResult {
  resultCount: number;
  results: Array<{
    trackId: number;
    trackName: string;
    artistName: string;
    collectionName?: string;
    isStreamable?: boolean;
  }>;
}

interface SonosState {
  playbackState?: string;
  currentTrack?: {
    title?: string;
    trackName?: string;
    artist?: string;
    artistName?: string;
    creator?: string;
  };
  currenttrack?: {
    title?: string;
    trackName?: string;
    artist?: string;
    artistName?: string;
    creator?: string;
  };
}

interface ZonePlayer {
  uuid: string;
  roomName: string;
}

interface ZoneInfo {
  coordinator: ZonePlayer;
  members: ZonePlayer[];
}

// Filter out covers, lullabies, karaoke, etc.
const UNWANTED_KEYWORDS = [
  'lullaby', 'lullabies', 'karaoke', 'tribute', 'cover', 'instrumental',
  'kids', 'baby', 'babies', 'nursery', 'toddler', 'children',
  'made famous', 'in the style of', 'originally performed',
  '8-bit', '8 bit', 'piano version', 'acoustic cover',
  'ringtone', 'workout', 'fitness'
];

function isUnwantedVersion(track: { trackName: string; artistName: string; collectionName?: string }, wantedArtist: string): boolean {
  const trackLower = track.trackName.toLowerCase();
  const artistLower = track.artistName.toLowerCase();
  const albumLower = (track.collectionName || '').toLowerCase();
  const wantedLower = wantedArtist.toLowerCase();

  // Check if any unwanted keywords appear
  for (const keyword of UNWANTED_KEYWORDS) {
    if (trackLower.includes(keyword) || artistLower.includes(keyword) || albumLower.includes(keyword)) {
      return true;
    }
  }

  // If the artist name is very different, it's probably a cover
  if (!artistLower.includes(wantedLower) && !wantedLower.includes(artistLower)) {
    // Allow if it's a "feat." or "&" collaboration
    if (!artistLower.includes('feat') && !artistLower.includes('&') && !artistLower.includes('with ')) {
      return true;
    }
  }

  return false;
}

function scoreMatch(track: { trackName: string; artistName: string }, song: Song): number {
  let score = 0;
  const trackTitle = track.trackName.toLowerCase();
  const trackArtist = track.artistName.toLowerCase();
  const wantedTitle = song.title.toLowerCase();
  const wantedArtist = song.artist.toLowerCase();

  // Exact artist match is best
  if (trackArtist === wantedArtist) score += 100;
  else if (trackArtist.includes(wantedArtist)) score += 50;
  else if (wantedArtist.includes(trackArtist)) score += 40;

  // Exact title match
  if (trackTitle === wantedTitle) score += 100;
  else if (trackTitle.includes(wantedTitle)) score += 50;
  else if (wantedTitle.includes(trackTitle)) score += 40;

  // Penalize if title has extra stuff (remix, live, etc.)
  if (trackTitle.includes('remix')) score -= 20;
  if (trackTitle.includes('live')) score -= 10;
  if (trackTitle.includes('remaster')) score -= 5;
  if (trackTitle.includes('edit')) score -= 15;
  if (trackTitle.includes('version')) score -= 10;

  return score;
}

const DEBUG = process.env.DEBUG === '1';

function normalizeText(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .trim();
}

function stateMatchesSong(state: SonosState, song: Song): boolean {
  const current = state.currentTrack ?? state.currenttrack;
  const stateTitle = current?.title ?? current?.trackName ?? '';
  const stateArtist = current?.artist ?? current?.artistName ?? current?.creator ?? '';
  const wantedTitle = normalizeText(song.title);
  const wantedArtist = normalizeText(song.artist);
  const gotTitle = normalizeText(stateTitle);
  const gotArtist = normalizeText(stateArtist);
  if (!gotTitle || !gotArtist) return false;
  return gotTitle.includes(wantedTitle) && gotArtist.includes(wantedArtist);
}

async function sleep(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForExpectedTrack(
  client: SonosClient,
  room: string,
  song: Song,
  timeoutMs = 5000
): Promise<boolean> {
  const encodedRoom = encodeURIComponent(room);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const state = await client.request<SonosState>(`/${encodedRoom}/state`);
      if (stateMatchesSong(state, song)) {
        return true;
      }
    } catch {
      // Ignore transient state fetch failures during startup
    }
    await sleep(300);
  }
  return false;
}

function isPlayingState(state: SonosState): boolean {
  const playbackState = String(state.playbackState || '').toUpperCase();
  return playbackState === 'PLAYING' || playbackState === 'TRANSITIONING';
}

async function waitForExpectedTrackPlaying(
  client: SonosClient,
  room: string,
  song: Song,
  timeoutMs = 5000
): Promise<boolean> {
  const encodedRoom = encodeURIComponent(room);
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const state = await client.request<SonosState>(`/${encodedRoom}/state`);
      if (stateMatchesSong(state, song) && isPlayingState(state)) {
        return true;
      }
    } catch {
      // Ignore transient state fetch failures during startup
    }
    await sleep(300);
  }
  return false;
}

async function getCoordinatorUuid(
  client: SonosClient,
  room: string
): Promise<string | null> {
  try {
    const zones = await client.request<ZoneInfo[]>('/zones');
    const zone = zones.find((z) => {
      if (z.coordinator?.roomName === room) return true;
      return Array.isArray(z.members) && z.members.some((m) => m.roomName === room);
    });
    return zone?.coordinator?.uuid ?? null;
  } catch {
    return null;
  }
}

async function searchiTunesCandidates(song: Song): Promise<number[]> {
  const query = encodeURIComponent(`${song.artist} ${song.title}`);
  const url = `https://itunes.apple.com/search?media=music&limit=25&entity=song&term=${query}`;
  const blockedIds = getBlockedTrackIds();
  const storedReplacements = getSongReplacementTrackIds(song).filter(
    (id) => !blockedIds.has(id)
  );

  try {
    const response = await fetch(url);
    if (!response.ok) {
      if (DEBUG) console.log(`    [DEBUG] iTunes fetch failed: ${response.status}`);
      return [];
    }

    const data = (await response.json()) as iTunesSearchResult;
    if (DEBUG) console.log(`    [DEBUG] iTunes returned ${data.resultCount} results`);
    if (data.resultCount === 0) return [];

    // Filter and score results
    const afterStreamable = data.results.filter((t) => t.isStreamable !== false);
    const afterUnwanted = afterStreamable.filter((t) => !isUnwantedVersion(t, song.artist));
    // Filter out blocked tracks that previously failed
    const afterBlocked = afterUnwanted.filter((t) => !blockedIds.has(t.trackId));
    if (DEBUG) {
      console.log(`    [DEBUG] After streamable filter: ${afterStreamable.length}`);
      console.log(`    [DEBUG] After unwanted filter: ${afterUnwanted.length}`);
      console.log(`    [DEBUG] After blocklist filter: ${afterBlocked.length}`);
      if (afterBlocked.length === 0 && afterStreamable.length > 0) {
        console.log(`    [DEBUG] All filtered out. Top results were:`);
        afterStreamable.slice(0, 3).forEach((t) => {
          console.log(`      - ${t.artistName} - ${t.trackName}`);
        });
      }
    }

    const scored = afterBlocked
      .map((t) => ({ track: t, score: scoreMatch(t, song) }))
      .sort((a, b) => b.score - a.score);

    const positive = scored.filter((c) => c.score > 0).map((c) => c.track.trackId);
    if (positive.length > 0) {
      const preferred = [...storedReplacements, ...positive].filter(
        (id, idx, arr) => arr.indexOf(id) === idx
      );
      if (DEBUG) {
        if (storedReplacements.length > 0) {
          console.log(`    [DEBUG] Found ${storedReplacements.length} stored replacements`);
        }
        console.log(`    [DEBUG] Found ${positive.length} positive matches, best ID: ${preferred[0]}`);
      }
      return preferred;
    }

    if (storedReplacements.length > 0) {
      if (DEBUG) {
        console.log(`    [DEBUG] Using stored replacement IDs: ${storedReplacements.join(', ')}`);
      }
      return storedReplacements;
    }

    // If no good match found, try original results but filter unwanted and blocked
    const fallback = data.results.find(
      (t) => t.isStreamable !== false && !isUnwantedVersion(t, song.artist) && !blockedIds.has(t.trackId)
    );
    if (DEBUG) console.log(`    [DEBUG] Using fallback: ${fallback ? fallback.trackId : 'none'}`);
    return fallback ? [fallback.trackId] : [];
  } catch (err) {
    if (DEBUG) console.log(`    [DEBUG] iTunes search error: ${err}`);
    return [];
  }
}

export async function clearQueue(
  client: SonosClient,
  room: string
): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/clearqueue`);
}

export async function addSongToQueue(
  client: SonosClient,
  room: string,
  song: Song,
  playNow = false
): Promise<QueueResult> {
  // First search iTunes to get track ID
  const candidates = await searchiTunesCandidates(song);
  const trackId = candidates[0];
  if (!trackId) {
    if (DEBUG) console.log(`    [DEBUG] No track ID found for ${song.artist} - ${song.title}`);
    return { song, success: false, error: 'Not found on iTunes' };
  }

  // Always enqueue to Sonos queue. For first track, seek to queue position 1 and play.
  // This avoids cases where "now" + "play" resumes previous transport instead.
  const action = 'queue';
  const encodedRoom = encodeURIComponent(room);
  const endpoint = `/${encodedRoom}/applemusic/${action}/song:${trackId}`;
  if (DEBUG) console.log(`    [DEBUG] Queueing via: ${endpoint}`);
  try {
    await client.requestNoResponse(endpoint);
    // Ensure playback starts from Sonos queue rather than previous source.
    if (playNow) {
      const coordinatorUuid = await getCoordinatorUuid(client, room);
      if (coordinatorUuid) {
        const queueUri = encodeURIComponent(`x-rincon-queue:${coordinatorUuid}#0`);
        try {
          await client.requestNoResponse(`/${encodedRoom}/setavtransporturi/${queueUri}`);
        } catch {
          // Continue; not all environments expose setavtransporturi reliably.
        }
      }
      try {
        await client.requestNoResponse(`/${encodedRoom}/trackseek/1`);
      } catch {
        // If trackseek fails, still attempt play as best effort.
      }
      await client.requestNoResponse(`/${encodedRoom}/play`);

      // Verify Sonos actually switched and started playing the requested track.
      // If not, force-switch with "now" as a fallback.
      let startedExpectedTrack = await waitForExpectedTrackPlaying(client, room, song);
      if (!startedExpectedTrack) {
        if (DEBUG) {
          console.log('    [DEBUG] Queue start verification failed, forcing now/play fallback');
        }
        await client.requestNoResponse(`/${encodedRoom}/applemusic/now/song:${trackId}`);
        await client.requestNoResponse(`/${encodedRoom}/play`);
        startedExpectedTrack = await waitForExpectedTrackPlaying(client, room, song, 5000);
      }

      // If the track becomes current but never reaches PLAYING, treat as unavailable.
      if (!startedExpectedTrack) {
        const isExpectedTrack = await waitForExpectedTrack(client, room, song, 1500);
        if (isExpectedTrack) {
          blockTrack(String(trackId), song.artist, song.title);
          return {
            song,
            success: false,
            error: 'Track appears unavailable (stays stopped)',
            trackId,
          };
        }
      }
    }
    if (DEBUG) console.log(`    [DEBUG] Queue request succeeded`);
    return { song, success: true, trackId };
  } catch (error) {
    if (DEBUG) console.log(`    [DEBUG] Queue request failed: ${error}`);
    return {
      song,
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error',
      trackId,
    };
  }
}

export async function addAlternateSongToQueue(
  client: SonosClient,
  room: string,
  song: Song,
  triedTrackIds: Set<number>
): Promise<QueueResult> {
  const candidates = await searchiTunesCandidates(song);
  const trackId = candidates.find((id) => !triedTrackIds.has(id));
  if (!trackId) {
    return { song, success: false, error: 'No alternate track found' };
  }

  const encodedRoom = encodeURIComponent(room);
  try {
    await client.requestNoResponse(
      `/${encodedRoom}/applemusic/queue/song:${trackId}`
    );
    return { song, success: true, trackId };
  } catch (error) {
    return {
      song,
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error',
      trackId,
    };
  }
}

export async function startPlayback(
  client: SonosClient,
  room: string
): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/play`);
}

export async function playFromStart(
  client: SonosClient,
  room: string
): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  // Try to seek to track 1, then play
  try {
    await client.requestNoResponse(`/${encodedRoom}/trackseek/1`);
  } catch {
    // trackseek might fail if queue is empty or other issues, continue anyway
  }
  await client.requestNoResponse(`/${encodedRoom}/play`);
}
