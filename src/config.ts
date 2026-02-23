import { existsSync, readFileSync } from 'fs';
import { homedir } from 'os';
import { join } from 'path';
import dotenv from 'dotenv';

dotenv.config();

export interface Config {
  anthropicApiKey?: string;
  openaiApiKey?: string;
  googleApiKey?: string;
  sonosApiUrl: string;
  defaultRoom?: string;
  defaultProvider: 'claude' | 'openai' | 'gemini';
  defaultCount: number;
}

interface FileConfig {
  sonosApiUrl?: string;
  defaultRoom?: string;
  defaultProvider?: 'claude' | 'openai' | 'gemini';
  defaultCount?: number;
}

function loadFileConfig(): FileConfig {
  const configPath = join(homedir(), '.config', 'sonos-playlist', 'config.json');
  if (existsSync(configPath)) {
    try {
      const content = readFileSync(configPath, 'utf-8');
      return JSON.parse(content) as FileConfig;
    } catch {
      return {};
    }
  }
  return {};
}

export function loadConfig(): Config {
  const fileConfig = loadFileConfig();

  return {
    anthropicApiKey: process.env.ANTHROPIC_API_KEY,
    openaiApiKey: process.env.OPENAI_API_KEY,
    googleApiKey: process.env.GOOGLE_API_KEY,
    sonosApiUrl:
      process.env.SONOS_API_URL ||
      fileConfig.sonosApiUrl ||
      'http://localhost:5005',
    defaultRoom: process.env.SONOS_DEFAULT_ROOM || fileConfig.defaultRoom,
    defaultProvider: fileConfig.defaultProvider || 'claude',
    defaultCount: fileConfig.defaultCount || 15,
  };
}

export function getApiKey(
  config: Config,
  provider: 'claude' | 'openai' | 'gemini'
): string {
  switch (provider) {
    case 'claude':
      if (!config.anthropicApiKey) {
        throw new Error('ANTHROPIC_API_KEY environment variable is not set');
      }
      return config.anthropicApiKey;
    case 'openai':
      if (!config.openaiApiKey) {
        throw new Error('OPENAI_API_KEY environment variable is not set');
      }
      return config.openaiApiKey;
    case 'gemini':
      if (!config.googleApiKey) {
        throw new Error('GOOGLE_API_KEY environment variable is not set');
      }
      return config.googleApiKey;
  }
}
