import { Command } from 'commander';
import { loadConfig } from './config.js';

export interface CliOptions {
  provider: 'claude' | 'openai' | 'gemini';
  room?: string;
  count: number;
  sonosApi: string;
  dryRun: boolean;
  monitor: boolean;
  listRooms: boolean;
  setup: boolean;
  prompt?: string;
}

export function parseArgs(): CliOptions {
  const config = loadConfig();
  const program = new Command();

  program
    .name('sonos')
    .description('AI-powered Sonos playlist generator (queues and exits by default)')
    .version('1.0.0')
    .argument('[prompt]', 'Natural language playlist description')
    .option(
      '-p, --provider <provider>',
      'AI provider: claude, openai, gemini',
      config.defaultProvider
    )
    .option('-r, --room <room>', 'Sonos speaker name', config.defaultRoom)
    .option(
      '-c, --count <number>',
      'Number of songs (10-20)',
      String(config.defaultCount)
    )
    .option('-s, --sonos-api <url>', 'Sonos HTTP API URL', config.sonosApiUrl)
    .option('-d, --dry-run', 'Preview playlist without playing', false)
    .option(
      '--monitor',
      'Keep running after queueing and auto-skip unavailable tracks (opt-in)',
      false
    )
    .option('-l, --list-rooms', 'Show available Sonos speakers', false)
    .option('--setup', 'Install and start node-sonos-http-api', false);

  program.parse();

  const options = program.opts();
  const prompt = program.args[0];

  const count = Math.min(20, Math.max(10, parseInt(options.count, 10) || 15));

  return {
    provider: options.provider as 'claude' | 'openai' | 'gemini',
    room: options.room,
    count,
    sonosApi: options.sonosApi,
    dryRun: options.dryRun,
    monitor: options.monitor,
    listRooms: options.listRooms,
    setup: options.setup,
    prompt,
  };
}
