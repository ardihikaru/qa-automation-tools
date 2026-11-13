// Test configuration and constants
export const config = {
  // Backend API
  // Override with QA_BACKEND_URL (e.g. https://qa-tool.mfardiansyah.id)
  backendUrl: process.env.QA_BACKEND_URL || 'http://localhost:8080',
  
  // Dashboard
  // Override with QA_DASHBOARD_URL (e.g. https://qa-tool.mfardiansyah.id)
  dashboardUrl: process.env.QA_DASHBOARD_URL || 'http://localhost:3000',
  
  // Impersonation tests (tests/api/impersonation.spec.ts); skipped when unset.
  // Impersonator = an Admin or Service account; target = a QA/QC user.
  impersonator: (process.env.E2E_IMPERSONATOR_USERNAME || '').toLowerCase(),
  impersonatorPassword: process.env.E2E_IMPERSONATOR_PASSWORD || '',
  impersonationTarget: (process.env.E2E_TARGET_USERNAME || '').toLowerCase(),

  // Timeouts
  apiTimeout: 10000,
  pageTimeout: 30000,
  
  // Expected data
  expectedSkills: ['max_length', 'required_field', 'data_persistence', 'multi_tab_save'],
};