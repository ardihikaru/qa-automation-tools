/**
 * Summarizer - Call Minimax AI to generate documentation
 */

import * as fs from 'fs';
import * as path from 'path';

export interface MinimaxConfig {
  api_key: string;
  base_url?: string;
}

export interface SummarizationRequest {
  flow_name: string;
  flow_description: string;
  role: string;
  steps: string[];
  screenshots: string[];
}

export interface SummarizationResponse {
  feature_description: string;
  user_flow: string;
  edge_cases: string[];
  key_points: string[];
}

/**
 * Call Minimax API for flow summarization
 */
export async function callMinimax(
  request: SummarizationRequest,
  config: MinimaxConfig
): Promise<SummarizationResponse> {
  const prompt = `
You are a technical writer. Generate documentation for this flow.

Flow: ${request.flow_name}
Role: ${request.role}
Description: ${request.flow_description}
Steps:
${request.steps.map((s, i) => `${i + 1}. ${s}`).join('\n')}

Generate:
1. Feature description (2-3 sentences)
2. User flow (numbered steps)
3. Edge cases (bullet list)
4. Key points (bullet list)
`;

  try {
    const response = await fetch(config.base_url || 'https://api.minimax.chat/v1/text/chatcompletion_pro', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${config.api_key}`,
      },
      body: JSON.stringify({
        model: 'abab6.5s-chat',
        messages: [
          { role: 'user', content: prompt },
        ],
        temperature: 0.7,
      }),
    });

    if (!response.ok) {
      throw new Error(`Minimax API error: ${response.status}`);
    }

    const data = await response.json() as { choices?: Array<{ message?: { content?: string } }> };
    const content = data.choices?.[0]?.message?.content || '';

    // Parse response (simple parsing - could be improved)
    const lines = content.split('\n').filter((l: string) => l.trim());
    
    return {
      feature_description: lines[0] || request.flow_description,
      user_flow: request.steps.join('\n'),
      edge_cases: lines.filter((l: string) => l.startsWith('-')).slice(0, 5),
      key_points: lines.filter((l: string) => l.startsWith('*')).slice(0, 5),
    };
  } catch (error) {
    console.error('❌ Minimax API error:', error);
    // Return fallback
    return {
      feature_description: request.flow_description,
      user_flow: request.steps.join('\n'),
      edge_cases: ['Error generating edge cases'],
      key_points: ['Error generating key points'],
    };
  }
}

/**
 * Generate markdown documentation from flow
 */
export async function generateDocumentation(
  flowName: string,
  flowDescription: string,
  role: string,
  steps: string[],
  config: MinimaxConfig,
  outputDir: string
): Promise<string> {
  // Call Minimax
  const summary = await callMinimax(
    {
      flow_name: flowName,
      flow_description: flowDescription,
      role,
      steps,
      screenshots: [],
    },
    config
  );

  // Generate markdown
  const markdown = generateMarkdown(flowName, role, summary);

  // Save to file
  const fileName = flowName.toLowerCase().replace(/\s+/g, '-');
  const filePath = path.join(outputDir, `${fileName}.md`);
  
  fs.writeFileSync(filePath, markdown, 'utf8');

  return filePath;
}

/**
 * Generate markdown content
 */
function generateMarkdown(
  flowName: string,
  role: string,
  summary: SummarizationResponse
): string {
  return `# ${flowName}

## Overview

${summary.feature_description}

## User Flow

${summary.user_flow}

## Role

This flow is available for: **${role}**

## Edge Cases

${summary.edge_cases.map(e => `- ${e}`).join('\n')}

## Key Points

${summary.key_points.map(k => `- ${k}`).join('\n')}

---

*Generated automatically by Playwright Documentation Generator*
`;
}

