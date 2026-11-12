/**
 * Summarizer - generate flow documentation through the AI gateway.
 *
 * Default provider is the production ai-manager (POST {manager_url}/v1/chat,
 * HTTP Basic auth, reply field `text`), reached over WireGuard.
 * MiniMax direct is kept as a fallback (provider: "minimax").
 */

import * as fs from 'fs';
import * as path from 'path';
import type { AIConfig } from '../../../lib/config';

/** @deprecated use AIConfig (provider: "minimax") */
export interface MinimaxConfig {
  api_key: string;
  base_url?: string;
}

export type SummarizerConfig = Partial<AIConfig> & { api_key?: string; base_url?: string };

const timeoutMs = (config: SummarizerConfig) => (config.timeout_seconds || 180) * 1000;

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

function buildPrompt(request: SummarizationRequest): string {
  return `
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
}

async function chatViaAIManager(prompt: string, config: SummarizerConfig): Promise<string> {
  const base = (config.manager_url || 'http://10.8.0.4:8099').replace(/\/+$/, '');
  if (!config.manager_login_id || !config.manager_secret_key) {
    throw new Error('AI_MANAGER_LOGIN_ID / AI_MANAGER_SECRET_KEY not set');
  }
  const auth = Buffer.from(`${config.manager_login_id}:${config.manager_secret_key}`).toString('base64');
  const response = await fetch(`${base}/v1/chat`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Basic ${auth}` },
    body: JSON.stringify({
      messages: [{ role: 'user', content: prompt }],
      provider: config.manager_provider || 'claude-v3',
      no_fallback: config.manager_no_fallback ?? true,
      model: config.manager_model || undefined,
      temperature: 0.7,
      caller: 'qa-automation-playwright-docs',
    }),
    signal: AbortSignal.timeout(timeoutMs(config)),
  });
  const data = (await response.json().catch(() => ({}))) as {
    text?: string;
    error?: string;
    provider?: string;
    fallback?: boolean;
  };
  if (!response.ok) {
    throw new Error(`ai-manager error ${response.status}: ${data.error || 'unknown'}`);
  }
  const pinned = config.manager_provider || 'claude-v3';
  if ((config.manager_no_fallback ?? true) && (data.fallback || (data.provider && data.provider !== pinned))) {
    throw new Error(`ai-manager answered by ${data.provider} but provider is pinned to ${pinned}`);
  }
  if (!data.text?.trim()) {
    throw new Error('ai-manager returned empty text');
  }
  return data.text;
}

async function chatViaMiniMax(prompt: string, config: SummarizerConfig): Promise<string> {
  const response = await fetch(config.base_url || 'https://api.minimax.io/v1/chat/completions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${config.api_key}` },
    body: JSON.stringify({
      model: config.model || 'MiniMax-M2.5',
      messages: [{ role: 'user', content: prompt }],
      temperature: 0.7,
    }),
    signal: AbortSignal.timeout(timeoutMs(config)),
  });
  if (!response.ok) {
    throw new Error(`Minimax API error: ${response.status}`);
  }
  const data = (await response.json()) as { choices?: Array<{ message?: { content?: string } }> };
  return data.choices?.[0]?.message?.content || '';
}

/**
 * Summarize a flow through the configured AI provider (ai-manager by default).
 */
export async function callAI(
  request: SummarizationRequest,
  config: SummarizerConfig
): Promise<SummarizationResponse> {
  const prompt = buildPrompt(request);
  const provider = config.provider || 'ai-manager';
  try {
    const content =
      provider === 'minimax' ? await chatViaMiniMax(prompt, config) : await chatViaAIManager(prompt, config);

    // Parse response (simple parsing - could be improved)
    const lines = content.split('\n').filter((l: string) => l.trim());
    return {
      feature_description: lines[0] || request.flow_description,
      user_flow: request.steps.join('\n'),
      edge_cases: lines.filter((l: string) => l.startsWith('-')).slice(0, 5),
      key_points: lines.filter((l: string) => l.startsWith('*')).slice(0, 5),
    };
  } catch (error) {
    console.error(`❌ AI (${provider}) error:`, error);
    return {
      feature_description: request.flow_description,
      user_flow: request.steps.join('\n'),
      edge_cases: ['Error generating edge cases'],
      key_points: ['Error generating key points'],
    };
  }
}

/** @deprecated use callAI; kept so old imports still compile (forces provider "minimax"). */
export async function callMinimax(
  request: SummarizationRequest,
  config: MinimaxConfig
): Promise<SummarizationResponse> {
  return callAI(request, { ...config, provider: 'minimax' });
}

/**
 * Generate markdown documentation from flow
 */
export async function generateDocumentation(
  flowName: string,
  flowDescription: string,
  role: string,
  steps: string[],
  config: SummarizerConfig,
  outputDir: string
): Promise<string> {
  const summary = await callAI(
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

