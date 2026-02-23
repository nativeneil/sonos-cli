export class SonosClient {
  constructor(private baseUrl: string) {}

  async checkConnection(): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseUrl}/zones`, {
        signal: AbortSignal.timeout(5000),
      });
      return response.ok;
    } catch {
      return false;
    }
  }

  async request<T>(path: string): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`);
    if (!response.ok) {
      throw new Error(`Sonos API error: ${response.status}`);
    }
    return response.json() as Promise<T>;
  }

  async requestNoResponse(path: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}${path}`);
    if (!response.ok) {
      throw new Error(`Sonos API error: ${response.status}`);
    }
  }
}
