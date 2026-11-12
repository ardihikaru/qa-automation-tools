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
  getAI,
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

  // Check if video recording is enabled
  const recordVideo = docsConfig.output.record_video || false;
  console.log(`🎥 Video recording: ${recordVideo ? 'enabled' : 'disabled'}`);

  try {
    // Launch browser
    console.log('🚀 Launching browser...');
    const browserConfig = docsConfig.browser;
    browser = await chromium.launch({
      headless: browserConfig.headless,
      args: browserConfig.args || [],
      executablePath: process.env.CHROME_PATH || '/usr/bin/google-chrome',
    });

    const context = await browser.newContext({
      viewport: docsConfig.browser.viewport,
      recordVideo: recordVideo ? {
        dir: path.join(docsConfig.output.video_dir, staging.url.replace(/https?:\/\//, '')),
      } : undefined,
    });

    const page = await context.newPage();

    // Load flow definitions
    const flows = loadFlows();
    console.log(`📋 Loaded ${flows.length} flows`);

    // Get roles from config
    const roles = docsConfig.flows.roles;

    // Map role names to flow roles
    const roleMapping: Record<string, string> = {
      'doctor_sp': 'doctor',
      'doctor_gp': 'doctor',
      'doctor_sp_og': 'doctor',
      'doctor_sp_kfr': 'doctor',
      'doctor_sp_tht': 'doctor',
      'doctor_radiolog': 'doctor',
      'doctor_professor': 'doctor',
      'nurse': 'nurse',
      'nurse_coordinator': 'nurse',
      'nurse_igd': 'nurse',
      'nurse_hd': 'nurse',
      'nurse_icu': 'nurse',
      'nurse_ok': 'nurse',
      'nurse_mcu': 'nurse',
      'admission': 'admission',
      'admission_rj': 'admission',
      'admission_coordinator': 'admission',
      'cashier': 'cashier',
      'admin': 'admin',
      'super_admin': 'super_admin',
      'pharmacy': 'pharmacy',
      'pharmacy_coordinator': 'pharmacy',
      'laboratory': 'laboratory',
      'lab_coordinator': 'laboratory',
      'radiographer': 'laboratory',
      'finance': 'admin',
      'marketing': 'admin',
      'medical_coordinator': 'admin',
      'case_manager': 'admin',
      'quality': 'admin',
      'bod': 'admin',
    };

    // Run each role's flows
    for (const roleName of roles) {
      const creds = getCredentials(roleName);

      if (!creds) {
        console.log(`⚠️ No credentials for role: ${roleName}`);
        continue;
      }

      // Get the flow role for this credential role
      const flowRole = roleMapping[roleName] || roleName;
      console.log(`\n👤 Processing role: ${roleName} (mapped to: ${flowRole})`);

      for (const flow of flows) {
        if (flow.role !== flowRole && roleName !== 'all') continue;

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
              ai: getAI(),
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