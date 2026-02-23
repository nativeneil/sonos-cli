import { SonosClient } from './client.js';

export interface SonosZone {
  coordinator: {
    roomName: string;
    uuid: string;
  };
  members: Array<{
    roomName: string;
    uuid: string;
  }>;
}

export async function discoverRooms(client: SonosClient): Promise<string[]> {
  const zones = await client.request<SonosZone[]>('/zones');
  const rooms: string[] = [];

  for (const zone of zones) {
    rooms.push(zone.coordinator.roomName);
    for (const member of zone.members) {
      if (member.roomName !== zone.coordinator.roomName) {
        rooms.push(member.roomName);
      }
    }
  }

  return [...new Set(rooms)].sort();
}

export async function getDefaultRoom(client: SonosClient): Promise<string> {
  const zones = await client.request<SonosZone[]>('/zones');
  if (zones.length === 0) {
    throw new Error('No Sonos speakers found');
  }
  return zones[0].coordinator.roomName;
}
