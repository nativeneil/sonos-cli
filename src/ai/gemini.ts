import { Song } from './index.js';

export async function generatePlaylistGemini(
  apiKey: string,
  prompt: string,
  count: number
): Promise<Song[]> {
  const fullPrompt = `You are a music expert. Generate a playlist of exactly ${count} songs for: "${prompt}"

Return ONLY a JSON array of songs, no other text. Each song should have "title" and "artist" fields.
Example: [{"title": "Blue in Green", "artist": "Miles Davis"}, {"title": "Take Five", "artist": "Dave Brubeck"}]

Return only the JSON array, no explanation or markdown formatting.`;

  const response = await fetch(
    `https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent?key=${apiKey}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        contents: [{ parts: [{ text: fullPrompt }] }],
        generationConfig: {
          maxOutputTokens: 2048,
        },
      }),
    }
  );

  if (!response.ok) {
    const error = await response.text();
    throw new Error(`Gemini API error: ${response.status} - ${error}`);
  }

  const data = (await response.json()) as {
    candidates: Array<{ content: { parts: Array<{ text: string }> } }>;
  };
  const text = data.candidates[0]?.content?.parts[0]?.text || '';

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
