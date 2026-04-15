/**
 * Main entry point for Playwright Documentation Generator
 * Uses centralized config.yaml
 */

import { chromium, Browser, Page } from 'playwright';
import * as path from 'path';
import {
  loadConfig,
  getStaging,
  getCredentials,
  getPlaywrightDocs,
  getAllCredentials,
  getBackendURL,
} from '../../lib/config';

// Flow definitions
import { FlowDefinition, loadFlows } from './flows/flowLoader';
import { recordFlow, FlowConfig } from './recorder/recordFlow';

/**
 * Main execution
 */
async function main(): Promise<void> {
  console.log('🎬 Playwright Documentation Generator');
  console.log('=====================================');

  // Load configuration
  const config = loadConfig();
  const staging = getStaging();
  const docsConfig = getPlaywrightDocs();
  const allCreds = getAllCredentials();

  console.log(`📚 Using staging: ${staging.url}`);
  console.log(`📁 Output: ${docsConfig.output.docs_dir}`);

  let browser: Browser | null = null;

  try {
    // Launch browser
    console.log('🚀 Launching browser...');
    browser = await chromium.launch({
      headless: docsConfig.browser.headless,
      args: docsConfig.browser.args || [],
    });

    const context = await browser.newContext({
      viewport: docsConfig.browser.viewport,
    });

    const page = await context.newPage();

    // Load flow definitions
    const flows = loadFlows();
    console.log(`📋 Loaded ${flows.length} flows`);

    // Get roles from config
    const roles = docsConfig.flows.roles;

    // Run each role's flows
    for (const roleName of roles) {
      const creds = getCredentials(roleName);

      if (!creds) {
        console.log(`⚠️ No credentials for role: ${roleName}`);
        continue;
      }

      console.log(`\n👤 Processing role: ${roleName}`);

      for (const flow of flows) {
        if (flow.role !== roleName && roleName !== 'all') continue;

        console.log(`  📝 Running flow: ${flow.name}`);

        try {
          // Create flow config
          const flowConfig: FlowConfig = {
            staging: {
              url: staging.url,
              timeout: staging.timeout,
            },
            api: {
              backend_url: getBackendURL(),
              minimax_api_key: config.backend?.ai?.api_key || '',
            },
          };

          await recordFlow(page, flow, creds, flowConfig);
          console.log(`  ✅ Completed: ${flow.name}`);
        } catch (error) {
          console.error(`  ❌ Failed: ${error}`);
          if (!docsConfig.flows.continue_on_error) {
            break;
          }
        }
      }
    }

    console.log('\n✅ Documentation generation complete!');

  } catch (error) {
    console.error('❌ Error:', error);
    process.exit(1);
  } finally {
    if (browser) {
      await browser.close();
    }
  }
}

// Run if executed directly
main().catch(console.error);

export { main };