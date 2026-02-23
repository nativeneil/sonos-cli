import chalk from 'chalk';
import {
  generatePlaylistAllProviders,
  generateMoreSongs,
  Song,
  RankedSong,
  APIKeys,
} from '../ai/index.js';
import { generatePlaylistGemini } from '../ai/gemini.js';
import { SonosClient } from '../sonos/client.js';
import { pause } from '../sonos/controls.js';
import { startPlaybackMonitor } from '../sonos/monitor.js';
import {
  clearQueue,
  addSongToQueue,
  addAlternateSongToQueue,
} from '../sonos/queue.js';
import { setTrackReplacement } from '../storage/index.js';

export interface GeneratorOptions {
  keys: APIKeys;
  prompt: string;
  room: string;
  dryRun: boolean;
  client: SonosClient;
  monitor: boolean;
}

const MIN_SONGS = 30;
const SONGS_PER_PROVIDER = 20;
const MAX_RETRIES = 5;
const ALT_RETRY_COOLDOWN_MS = 30_000;
const FAST_START_TIMEOUT_MS = 10_000;

function songKey(song: Song): string {
  return `${song.artist.toLowerCase()}:::${song.title.toLowerCase()}`;
}

async function generateFastStartSongs(
  keys: APIKeys,
  prompt: string
): Promise<Song[]> {
  if (!keys.google) return [];

  const timeout = new Promise<never>((_, reject) => {
    setTimeout(() => {
      reject(new Error(`timed out after ${FAST_START_TIMEOUT_MS / 1000}s`));
    }, FAST_START_TIMEOUT_MS);
  });

  return Promise.race([
    generatePlaylistGemini(keys.google, prompt, 3),
    timeout,
  ]);
}

export async function generateAndPlay(
  options: GeneratorOptions
): Promise<void> {
  const { keys, prompt, room, dryRun, client, monitor } = options;

  let playbackStarted = false;
  let fastStartSong: Song | null = null;
  const existingKeys = new Set<string>();
  const triedTrackIdsBySong = new Map<string, Set<number>>();
  const lastAltAttemptAt = new Map<string, number>();
  let monitorHandle: { stop: () => void; done: Promise<void> } | null = null;

  const handleUnplayable = async (song: Song, trackId: string | null): Promise<void> => {
    const key = songKey(song);
    console.log(
      chalk.yellow(`Detected unplayable track: ${song.artist} - ${song.title}${trackId ? ` (ID: ${trackId}, now blocked)` : ''}`)
    );
    const now = Date.now();
    const lastAttempt = lastAltAttemptAt.get(key) ?? 0;
    if (now - lastAttempt < ALT_RETRY_COOLDOWN_MS) {
      return;
    }
    lastAltAttemptAt.set(key, now);
    const tried = triedTrackIdsBySong.get(key) ?? new Set<number>();
    if (trackId) {
      tried.add(parseInt(trackId, 10));
    }
    const alt = await addAlternateSongToQueue(client, room, song, tried);
    if (alt.success && alt.trackId !== undefined) {
      tried.add(alt.trackId);
      triedTrackIdsBySong.set(key, tried);
      if (trackId) {
        setTrackReplacement(trackId, String(alt.trackId));
      }
      console.log(
        chalk.yellow(
          `Unavailable track replaced with alternate: ${song.artist} - ${song.title}`
        )
      );
    } else {
      console.log(
        chalk.red(
          `No alternate found for: ${song.artist} - ${song.title}`
        )
      );
    }
  };

  const ensureMonitor = (): void => {
    if (!monitor) {
      return;
    }
    if (monitorHandle) {
      return;
    }
    monitorHandle = startPlaybackMonitor(client, room, {
      onUnplayable: handleUnplayable,
      useTimer: false,
    });
    console.log(
      chalk.gray(
        'Monitoring playback for unavailable tracks (Ctrl+C to stop)...'
      )
    );
  };

  let playlistPromise: Promise<Array<Song & { votes?: number }>> | null = null;

  if (!dryRun) {
    console.log(chalk.blue('Generating fast-start songs (Gemini 3 Flash)...'));
    // Start all async operations in parallel for faster startup
    const fastStartPromise = generateFastStartSongs(keys, prompt);
    playlistPromise = generatePlaylistAllProviders(
      keys,
      prompt,
      SONGS_PER_PROVIDER
    );
    const clearPromise = (async () => {
      try {
        await pause(client, room);
      } catch {
        // Ignore pause failures
      }
      await clearQueue(client, room);
    })();

    await clearPromise;

    try {
      const fastStartCandidates = await fastStartPromise;
      if (fastStartCandidates.length === 0) {
        console.log(chalk.yellow('Fast-start unavailable; continuing'));
      } else {
        for (const candidate of fastStartCandidates) {
          // Use playNow=true to switch transport to queue and start playing
          const result = await addSongToQueue(client, room, candidate, true);
          if (!result.success) {
            continue;
          }
          const key = songKey(candidate);
          if (result.trackId !== undefined) {
            const tried = triedTrackIdsBySong.get(key) ?? new Set<number>();
            tried.add(result.trackId);
            triedTrackIdsBySong.set(key, tried);
          }
          existingKeys.add(key);
          playbackStarted = true;
          fastStartSong = candidate;
          console.log(
            chalk.green(
              `  -> Now playing: ${candidate.artist} - ${candidate.title}\n`
            )
          );
          ensureMonitor();
          break;
        }
        if (!playbackStarted) {
          console.log(
            chalk.yellow('Fast-start songs queued but none were playable')
          );
        }
      }
    } catch (error) {
      console.log(
        chalk.yellow(
          `Fast-start skipped (${(error as Error).message}); continuing`
        )
      );
    }
  }

  // Generate playlist from all AI providers
  console.log(
    chalk.blue(`Generating playlist from all providers for: "${prompt}"`)
  );
  let songs: Array<Song & { votes?: number }> = playlistPromise
    ? await playlistPromise
    : await generatePlaylistAllProviders(keys, prompt, SONGS_PER_PROVIDER);
  console.log(
    chalk.green(`Generated ${songs.length} unique songs from all providers\n`)
  );

  // Display initial playlist
  console.log(chalk.bold('Initial playlist:'));
  songs.forEach((song, i) => {
    const votes = song.votes || 1;
    const voteIndicator = votes > 1 ? chalk.yellow(` [${votes}]`) : '';
    console.log(chalk.white(`  ${i + 1}. ${song.artist} - ${song.title}`) + voteIndicator);
  });
  console.log();

  if (dryRun) {
    console.log(chalk.yellow('Dry run - not queueing or playing'));
    return;
  }

  if (playbackStarted) {
    console.log(
      chalk.blue('Fast-start playing; appending generated songs to the queue...')
    );
  }

  // Queue songs and track which ones succeed
  const queuedSongs: Song[] = [];
  const failedSongs: Song[] = [];
  let retryCount = 0;

  while (queuedSongs.length < MIN_SONGS && retryCount < MAX_RETRIES) {
    if (retryCount > 0) {
      console.log(chalk.blue(`\nGenerating more songs (attempt ${retryCount + 1})...`));
      const moreSongs = await generateMoreSongs(
        keys,
        prompt,
        [...queuedSongs, ...failedSongs, ...(fastStartSong ? [fastStartSong] : [])],
        SONGS_PER_PROVIDER
      );
      if (moreSongs.length === 0) {
        console.log(chalk.yellow('Could not generate more unique songs'));
        break;
      }
      songs = moreSongs;
      console.log(chalk.green(`Generated ${songs.length} more songs\n`));
    }

    console.log(chalk.blue('Adding songs to queue...\n'));

    for (const song of songs) {
      const key = songKey(song);
      if (existingKeys.has(key)) {
        continue;
      }
      process.stdout.write(
        chalk.gray(`  Searching: ${song.artist} - ${song.title}... `)
      );
      // Use playNow=true for first song to switch transport to queue
      const shouldPlayNow = !playbackStarted;
      const result = await addSongToQueue(client, room, song, shouldPlayNow);

      if (result.success) {
        console.log(chalk.green('found'));
        queuedSongs.push(song);
        existingKeys.add(key);
        if (result.trackId !== undefined) {
          const tried = triedTrackIdsBySong.get(key) ?? new Set<number>();
          tried.add(result.trackId);
          triedTrackIdsBySong.set(key, tried);
        }
        if (shouldPlayNow) {
          playbackStarted = true;
          console.log(chalk.green('  -> Playback started!\n'));
          ensureMonitor();
        }
      } else {
        console.log(chalk.red('not found'));
        failedSongs.push(song);
      }
    }

    retryCount++;
  }

  // Summary
  console.log();
  console.log(chalk.bold(`Queued ${queuedSongs.length} songs on ${room}`));

  if (failedSongs.length > 0) {
    console.log(chalk.yellow(`\n${failedSongs.length} songs not found on Apple Music`));
  }

  if (!playbackStarted) {
    console.log(chalk.red('\nNo songs were queued, nothing to play'));
  }

  if (!monitor) {
    console.log(
      chalk.gray('Queueing complete. Exiting now (use --monitor to keep running).')
    );
    return;
  }

  if (monitorHandle) {
    await monitorHandle.done;
  }
}
