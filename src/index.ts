#!/usr/bin/env node

import chalk from 'chalk';
import { parseArgs } from './cli.js';
import { loadConfig } from './config.js';
import { runSetup, printSetupInstructions } from './setup.js';
import { SonosClient } from './sonos/client.js';
import { discoverRooms, getDefaultRoom } from './sonos/discovery.js';
import { generateAndPlay } from './playlist/generator.js';
import { APIKeys } from './ai/index.js';
import {
  play,
  pause,
  skip,
  previous,
  restart,
  volumeUp,
  volumeDown,
  volumeHigh,
  volumeLow,
  volumeSet,
  getVolume,
  repeat,
  shuffle,
} from './sonos/controls.js';

// Control commands that don't need AI
const CONTROL_COMMANDS: Record<string, (client: SonosClient, room: string) => Promise<void>> = {
  'play': async (client, room) => {
    await play(client, room);
    console.log(chalk.green('Playing'));
  },
  'pause': async (client, room) => {
    await pause(client, room);
    console.log(chalk.yellow('Paused'));
  },
  'stop': async (client, room) => {
    await pause(client, room);
    console.log(chalk.yellow('Stopped'));
  },
  'skip': async (client, room) => {
    await skip(client, room);
    console.log(chalk.green('Skipped to next track'));
  },
  'next': async (client, room) => {
    await skip(client, room);
    console.log(chalk.green('Skipped to next track'));
  },
  'previous': async (client, room) => {
    await previous(client, room);
    console.log(chalk.green('Back to previous track'));
  },
  'prev': async (client, room) => {
    await previous(client, room);
    console.log(chalk.green('Back to previous track'));
  },
  'back': async (client, room) => {
    await previous(client, room);
    console.log(chalk.green('Back to previous track'));
  },
  'again': async (client, room) => {
    await restart(client, room);
    console.log(chalk.green('Restarted track'));
  },
  'replay': async (client, room) => {
    await restart(client, room);
    console.log(chalk.green('Restarted track'));
  },
  'restart': async (client, room) => {
    await restart(client, room);
    console.log(chalk.green('Restarted track'));
  },
  'volume up': async (client, room) => {
    const vol = await volumeUp(client, room);
    console.log(chalk.green(`Volume: ${vol}`));
  },
  'volume down': async (client, room) => {
    const vol = await volumeDown(client, room);
    console.log(chalk.green(`Volume: ${vol}`));
  },
  'volume high': async (client, room) => {
    const vol = await volumeHigh(client, room);
    console.log(chalk.green(`Volume: ${vol}`));
  },
  'volume low': async (client, room) => {
    const vol = await volumeLow(client, room);
    console.log(chalk.green(`Volume: ${vol}`));
  },
  'volume': async (client, room) => {
    const vol = await getVolume(client, room);
    console.log(chalk.green(`Volume: ${vol}`));
  },
  'repeat': async (client, room) => {
    const mode = await repeat(client, room);
    console.log(chalk.green(`Repeat: ${mode}`));
  },
  'repeat all': async (client, room) => {
    await repeat(client, room, 'all');
    console.log(chalk.green('Repeat: all'));
  },
  'repeat one': async (client, room) => {
    await repeat(client, room, 'one');
    console.log(chalk.green('Repeat: one'));
  },
  'repeat off': async (client, room) => {
    await repeat(client, room, 'none');
    console.log(chalk.green('Repeat: off'));
  },
  'shuffle': async (client, room) => {
    const enabled = await shuffle(client, room);
    console.log(chalk.green(`Shuffle: ${enabled ? 'on' : 'off'}`));
  },
  'shuffle on': async (client, room) => {
    await shuffle(client, room, true);
    console.log(chalk.green('Shuffle: on'));
  },
  'shuffle off': async (client, room) => {
    await shuffle(client, room, false);
    console.log(chalk.green('Shuffle: off'));
  },
};

function parseVolumeNumber(input: string): number | null {
  const match = input.match(/^volume\s+(\d+)$/i);
  if (match) {
    return parseInt(match[1], 10);
  }
  return null;
}

async function main(): Promise<void> {
  const options = parseArgs();
  const config = loadConfig();

  // Handle setup command
  if (options.setup) {
    await runSetup();
    return;
  }

  // Create Sonos client
  const client = new SonosClient(options.sonosApi);

  // Check Sonos connection (skip for dry-run)
  if (!options.dryRun) {
    const connected = await client.checkConnection();
    if (!connected) {
      printSetupInstructions();
      process.exit(1);
    }
  }

  // Handle list-rooms command
  if (options.listRooms) {
    const connected = await client.checkConnection();
    if (!connected) {
      printSetupInstructions();
      process.exit(1);
    }
    const rooms = await discoverRooms(client);
    console.log(chalk.bold('Available Sonos speakers:'));
    rooms.forEach((room) => console.log(`  - ${room}`));
    return;
  }

  // Need a prompt
  if (!options.prompt) {
    console.error(
      chalk.red('Please provide a command or playlist description, e.g.:')
    );
    console.error(chalk.cyan('  sonos "relaxing jazz for Sunday morning"'));
    console.error(chalk.cyan('  sonos play'));
    console.error(chalk.cyan('  sonos volume up'));
    console.error(chalk.gray('\nOr use --help for more options'));
    process.exit(1);
  }

  // Determine room
  let room = options.room;
  if (!room) {
    if (options.dryRun) {
      room = 'Living Room';
    } else {
      try {
        room = await getDefaultRoom(client);
      } catch (error) {
        console.error(chalk.red((error as Error).message));
        process.exit(1);
      }
    }
  }

  // Check for control commands
  const promptLower = options.prompt.toLowerCase().trim();

  // Check for "volume <number>" pattern
  const volumeNum = parseVolumeNumber(promptLower);
  if (volumeNum !== null) {
    const vol = await volumeSet(client, room, volumeNum);
    console.log(chalk.green(`Volume: ${vol}`));
    return;
  }

  // Check for other control commands
  const controlHandler = CONTROL_COMMANDS[promptLower];
  if (controlHandler) {
    try {
      await controlHandler(client, room);
    } catch (error) {
      console.error(chalk.red(`Error: ${(error as Error).message}`));
      process.exit(1);
    }
    return;
  }

  // Otherwise, treat as playlist generation
  const keys: APIKeys = {
    anthropic: config.anthropicApiKey,
    openai: config.openaiApiKey,
    google: config.googleApiKey,
  };

  if (!keys.anthropic && !keys.openai && !keys.google) {
    console.error(chalk.red('No API keys configured.'));
    console.error(chalk.white('Set at least one of:'));
    console.error(chalk.cyan('  ANTHROPIC_API_KEY'));
    console.error(chalk.cyan('  OPENAI_API_KEY'));
    console.error(chalk.cyan('  GOOGLE_API_KEY'));
    process.exit(1);
  }

  const providers: string[] = [];
  if (keys.anthropic) providers.push('Claude');
  if (keys.openai) providers.push('OpenAI');
  if (keys.google) providers.push('Gemini');
  console.log(chalk.gray(`Using providers: ${providers.join(', ')}\n`));

  if (!options.dryRun) {
    console.log(chalk.gray(`Using speaker: ${room}\n`));
  }

  try {
    await generateAndPlay({
      keys,
      prompt: options.prompt,
      room,
      dryRun: options.dryRun,
      client,
      monitor: options.monitor,
    });
  } catch (error) {
    console.error(chalk.red(`Error: ${(error as Error).message}`));
    process.exit(1);
  }
}

main();
