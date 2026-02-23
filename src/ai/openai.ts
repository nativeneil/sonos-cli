import { Song } from './index.js';

export async function generatePlaylistOpenAI(
  apiKey: string,
  prompt: string,
  count: number
): Promise<Song[]> {
  const systemPrompt = `You are a music expert. Generate playlists based on user requests.
Return ONLY a JSON array of songs, no other text. Each song should have "title" and "artist" fields.
Example: [{"title": "Blue in Green", "artist": "Miles Davis"}, {"title": "Take Five", "artist": "Dave Brubeck"}]`;

  const userPrompt = `Generate a playlist of exactly ${count} songs for: "${prompt}"
Return only the JSON array, no explanation.`;

  const response = await fetch('https://api.openai.com/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      model: 'gpt-5.2',
      messages: [
        { role: 'system', content: systemPrompt },
        { role: 'user', content: userPrompt },
      ],
      max_tokens: 2048,
    }),
  });

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`OpenAI API error: ${response.status} - ${error}`);
  }

  const data = (await response.json()) as {
    choices: Array<{ message: { content: string } }>;
  };
  const text = data.choices[0]?.message?.content || '';

  return parsePlaylistResponse(text);
}

function parsePlaylistResponse(text: string): Song[] {
  // Try direct JSON parse first
  try {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) {
      return parsed.filter(
        (s): s is Song =>
          typeof s === 'object' &&
          s !== null &&
          typeof s.title === 'string' &&
          typeof s.artist === 'string'
      );
    }
  } catch {
    // Fall through to regex extraction
  }

  // Try to extract JSON array from text
  const jsonMatch = text.match(/\[[\s\S]*\]/);
  if (jsonMatch) {
    try {
      const parsed = JSON.parse(jsonMatch[0]);
      if (Array.isArray(parsed)) {
        return parsed.filter(
          (s): s is Song =>
            typeof s === 'object' &&
            s !== null &&
            typeof s.title === 'string' &&
            typeof s.artist === 'string'
        );
      }
    } catch {
      // Continue to fallback
    }
  }

  throw new Error('Failed to parse playlist from AI response');
}
