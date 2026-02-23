import { SonosClient } from './client.js';

interface PlayerState {
  volume: number;
  mute: boolean;
  playbackState: string;
  playMode: {
    repeat: string;
    shuffle: boolean;
  };
}

async function getState(client: SonosClient, room: string): Promise<PlayerState> {
  const encodedRoom = encodeURIComponent(room);
  const state = await client.request<PlayerState>(`/${encodedRoom}/state`);
  return state;
}

export async function volumeUp(client: SonosClient, room: string): Promise<number> {
  const state = await getState(client, room);
  const current = state.volume;
  // If on a multiple of 5, add 5. Otherwise round up to next multiple of 5.
  const newVolume = current % 5 === 0
    ? Math.min(100, current + 5)
    : Math.min(100, Math.ceil(current / 5) * 5);
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/volume/${newVolume}`);
  return newVolume;
}

export async function volumeDown(client: SonosClient, room: string): Promise<number> {
  const state = await getState(client, room);
  const current = state.volume;
  // If on a multiple of 5, subtract 5. Otherwise round down to previous multiple of 5.
  const newVolume = current % 5 === 0
    ? Math.max(0, current - 5)
    : Math.max(0, Math.floor(current / 5) * 5);
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/volume/${newVolume}`);
  return newVolume;
}

export async function volumeSet(client: SonosClient, room: string, level: number): Promise<number> {
  const newVolume = Math.max(0, Math.min(100, level));
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/volume/${newVolume}`);
  return newVolume;
}

export async function volumeHigh(client: SonosClient, room: string): Promise<number> {
  return volumeSet(client, room, 60);
}

export async function volumeLow(client: SonosClient, room: string): Promise<number> {
  return volumeSet(client, room, 10);
}

export async function getVolume(client: SonosClient, room: string): Promise<number> {
  const state = await getState(client, room);
  return state.volume;
}

export async function play(client: SonosClient, room: string): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/play`);
}

export async function pause(client: SonosClient, room: string): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/pause`);
}

export async function skip(client: SonosClient, room: string): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/next`);
}

export async function previous(client: SonosClient, room: string): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/previous`);
}

export async function restart(client: SonosClient, room: string): Promise<void> {
  const encodedRoom = encodeURIComponent(room);
  await client.requestNoResponse(`/${encodedRoom}/seek/0`);
}

export async function repeat(client: SonosClient, room: string, mode?: 'all' | 'one' | 'none'): Promise<string> {
  const encodedRoom = encodeURIComponent(room);
  if (mode) {
    await client.requestNoResponse(`/${encodedRoom}/repeat/${mode}`);
    return mode;
  }
  // Toggle through modes: none -> all -> one -> none
  const state = await getState(client, room);
  const current = state.playMode.repeat;
  const next = current === 'none' ? 'all' : current === 'all' ? 'one' : 'none';
  await client.requestNoResponse(`/${encodedRoom}/repeat/${next}`);
  return next;
}

export async function shuffle(client: SonosClient, room: string, enabled?: boolean): Promise<boolean> {
  const encodedRoom = encodeURIComponent(room);
  if (enabled !== undefined) {
    await client.requestNoResponse(`/${encodedRoom}/shuffle/${enabled ? 'on' : 'off'}`);
    return enabled;
  }
  // Toggle
  const state = await getState(client, room);
  const newState = !state.playMode.shuffle;
  await client.requestNoResponse(`/${encodedRoom}/shuffle/${newState ? 'on' : 'off'}`);
  return newState;
}
