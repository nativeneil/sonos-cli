import { generatePlaylistClaude } from './claude.js';
import { generatePlaylistOpenAI } from './openai.js';
import { generatePlaylistGemini } from './gemini.js';

export interface Song {
  title: string;
  artist: string;
}

export type AIProvider = 'claude' | 'openai' | 'gemini';

export interface APIKeys {
  anthropic?: string;
  openai?: string;
  google?: string;
}

export async function generatePlaylist(
  provider: AIProvider,
  apiKey: string,
  prompt: string,
  count: number
): Promise<Song[]> {
  switch (provider) {
    case 'claude':
      return generatePlaylistClaude(apiKey, prompt, count);
    case 'openai':
      return generatePlaylistOpenAI(apiKey, prompt, count);
    case 'gemini':
      return generatePlaylistGemini(apiKey, prompt, count);
  }
}

function songKey(song: Song): string {
  return `${song.artist.toLowerCase()}:::${song.title.toLowerCase()}`;
}

interface RankedSong extends Song {
  votes: number;
}

function dedupeAndRankSongs(songs: Song[]): RankedSong[] {
  const counts = new Map<string, { song: Song; votes: number }>();

  for (const song of songs) {
    const key = songKey(song);
    const existing = counts.get(key);
    if (existing) {
      existing.votes++;
    } else {
      counts.set(key, { song, votes: 1 });
    }
  }

  // Sort by votes (descending), then by original order
  return Array.from(counts.values())
    .sort((a, b) => b.votes - a.votes)
    .map(({ song, votes }) => ({ ...song, votes }));
}

export { RankedSong };

export async function generatePlaylistAllProviders(
  keys: APIKeys,
  prompt: string,
  countPerProvider: number
): Promise<RankedSong[]> {
  const promises: Promise<Song[]>[] = [];
  const providerNames: string[] = [];

  if (keys.anthropic) {
    promises.push(
      generatePlaylistClaude(keys.anthropic, prompt, countPerProvider)
        .catch(() => [])
    );
    providerNames.push('claude');
  }
  if (keys.openai) {
    promises.push(
      generatePlaylistOpenAI(keys.openai, prompt, countPerProvider)
        .catch(() => [])
    );
    providerNames.push('openai');
  }
  if (keys.google) {
    promises.push(
      generatePlaylistGemini(keys.google, prompt, countPerProvider)
        .catch(() => [])
    );
    providerNames.push('gemini');
  }

  if (promises.length === 0) {
    throw new Error('No API keys configured');
  }

  const results = await Promise.all(promises);
  const allSongs = results.flat();
  return dedupeAndRankSongs(allSongs);
}

export async function generateMoreSongs(
  keys: APIKeys,
  prompt: string,
  existingSongs: Song[],
  count: number
): Promise<Song[]> {
  const existingKeys = new Set(existingSongs.map(songKey));
  const excludeList = existingSongs
    .slice(0, 10)
    .map((s) => `${s.artist} - ${s.title}`)
    .join(', ');

  const newPrompt = `${prompt}. Exclude these songs: ${excludeList}. Give me different songs.`;

  // Try each provider until we get new songs
  const providers: Array<{ key?: string; fn: (key: string, prompt: string, count: number) => Promise<Song[]> }> = [
    { key: keys.anthropic, fn: generatePlaylistClaude },
    { key: keys.openai, fn: generatePlaylistOpenAI },
    { key: keys.google, fn: generatePlaylistGemini },
  ];

  for (const { key, fn } of providers) {
    if (!key) continue;
    try {
      const songs = await fn(key, newPrompt, count);
      const newSongs = songs.filter((s) => !existingKeys.has(songKey(s)));
      if (newSongs.length > 0) {
        return newSongs;
      }
    } catch {
      continue;
    }
  }

  return [];
}
